package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	"cosmossdk.io/log"
	"google.golang.org/protobuf/proto"

	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	"lumen/crypto/pqc/dilithium"
	pqcsigning "lumen/x/pqc/signing"
	pqctypes "lumen/x/pqc/types"
)

type pqcAccountKeeper interface {
	GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
}

type pqcKeeper interface {
	GetParams(ctx context.Context) pqctypes.Params
	GetAccountPQC(ctx context.Context, addr sdk.AccAddress) (pqctypes.AccountPQC, bool, error)
	Scheme() dilithium.Scheme
}

type pqcSignatureProvider interface {
	GetPQCSignatures() map[string]*pqctypes.PQCSignatureEntry
}

// PQCDualSignDecorator enforces PQC signatures for every transaction signed by
// an externally owned account (EOA). Module accounts never sign transactions,
// so they are unaffected.
type PQCDualSignDecorator struct {
	logger log.Logger
	ak     pqcAccountKeeper
	pqc    pqcKeeper
	cdc    codec.Codec
}

func NewPQCDualSignDecorator(
	logger log.Logger,
	ak pqcAccountKeeper,
	pqc pqcKeeper,
	cdc codec.Codec,
) PQCDualSignDecorator {
	return PQCDualSignDecorator{
		logger: logger,
		ak:     ak,
		pqc:    pqc,
		cdc:    cdc,
	}
}

func (d PQCDualSignDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if ctx.BlockHeight() <= 1 {
		return next(ctx, tx, simulate)
	}

	params := d.pqc.GetParams(ctx)
	if params.Policy == pqctypes.PqcPolicy_PQC_POLICY_DISABLED {
		return next(ctx, tx, simulate)
	}

	msgs := tx.GetMsgs()
	if len(msgs) == 0 {
		return next(ctx, tx, simulate)
	}

	authTx, ok := tx.(authsigning.Tx)
	if !ok {
		return ctx, fmt.Errorf("pqc: tx does not implement authsigning.Tx")
	}

	sigV2s, err := authTx.GetSignaturesV2()
	if err != nil {
		return ctx, errorsmod.Wrap(err, "pqc: unable to load signatures")
	}

	rawSigners, err := authTx.GetSigners()
	if err != nil {
		return ctx, errorsmod.Wrap(err, "pqc: unable to derive signers")
	}
	if len(rawSigners) != len(sigV2s) {
		return ctx, fmt.Errorf("pqc: signer/signature mismatch %d != %d", len(rawSigners), len(sigV2s))
	}
	signers := make([]sdk.AccAddress, len(rawSigners))
	for i, bz := range rawSigners {
		signers[i] = sdk.AccAddress(bz)
	}

	pqcSigs, sigsAvailable, err := d.extractPQCSigs(tx)
	if err != nil {
		return ctx, err
	}

	txRaw, err := d.txRawFromContext(ctx)
	if err != nil {
		return ctx, err
	}

	txHashBytes := computeTxHashBytes(ctx, txRaw)
	scheme := d.pqc.Scheme()
	required := params.Policy == pqctypes.PqcPolicy_PQC_POLICY_REQUIRED

	for idx, signer := range signers {
		account := d.ak.GetAccount(ctx, signer)
		if account == nil {
			return ctx, fmt.Errorf("pqc: signer account %s not found", signer.String())
		}

		accountPQC, found, err := d.pqc.GetAccountPQC(ctx, signer)
		if err != nil {
			return ctx, errorsmod.Wrapf(err, "pqc: lookup account %s", signer.String())
		}

		if !found {
			if pqcDebugEnabled() {
				pqcDebugLogSigner(txHashBytes, signer, "missing_account_key", "")
			}
			if _, ok := pendingLinkForSigner(msgs, signer); ok {
				d.emitEvent(ctx, pqctypes.EventTypeMissing, signer.String(), map[string]string{
					pqctypes.AttributeKeyReason: "link_in_progress",
				})
				continue
			}
			if required {
				return ctx, errorsmod.Wrapf(pqctypes.ErrPQCRequired, "signer %s missing pqc key", signer.String())
			}
			d.emitEvent(ctx, pqctypes.EventTypeMissing, signer.String(), map[string]string{
				pqctypes.AttributeKeyReason: "no_account_key",
			})
			continue
		}

		if !strings.EqualFold(accountPQC.Scheme, params.MinScheme) {
			return ctx, errorsmod.Wrapf(pqctypes.ErrInvalidScheme, "signer %s uses %s requires %s", signer.String(), accountPQC.Scheme, params.MinScheme)
		}

		entry, ok := pqcSigs[signer.String()]
		if !ok {
			if pqcDebugEnabled() {
				pqcDebugLogSigner(txHashBytes, signer, "missing_signature", "")
			}
			if d.logger != nil {
				d.logger.Debug("pqc signature missing", "tx_hash", fmt.Sprintf("%X", txHashBytes), "signer", signer.String(), "sigs_available", sigsAvailable)
			}
			if required {
				if sigsAvailable {
					return ctx, errorsmod.Wrapf(pqctypes.ErrPQCRequired, "signer %s missing pqc signature", signer.String())
				}
				return ctx, pqctypes.ErrMissingExtension
			}
			d.emitEvent(ctx, pqctypes.EventTypeMissing, signer.String(), map[string]string{
				pqctypes.AttributeKeyReason: "signature_absent",
			})
			continue
		}

		if !strings.EqualFold(entry.Scheme, params.MinScheme) {
			return ctx, errorsmod.Wrapf(pqctypes.ErrInvalidScheme, "signer %s provided scheme %s requires %s", signer.String(), entry.Scheme, params.MinScheme)
		}

		if len(entry.PubKey) != scheme.PublicKeySize() {
			return ctx, errorsmod.Wrapf(pqctypes.ErrInvalidPubKeyFormat, "signer %s provided pubkey length %d expected %d", signer.String(), len(entry.PubKey), scheme.PublicKeySize())
		}
		pubKeyHash := sha256.Sum256(entry.PubKey)
		if len(accountPQC.PubKeyHash) != sha256.Size || !bytes.Equal(pubKeyHash[:], accountPQC.PubKeyHash) {
			return ctx, errorsmod.Wrapf(pqctypes.ErrPQCVerifyFailed, "signer %s pubkey hash mismatch", signer.String())
		}

		sigData, ok := sigV2s[idx].Data.(*signingtypes.SingleSignatureData)
		if !ok {
			return ctx, fmt.Errorf("pqc: unsupported signature data for signer %s", signer.String())
		}

		if sigData.SignMode != signingtypes.SignMode_SIGN_MODE_DIRECT {
			return ctx, fmt.Errorf("pqc: unsupported sign mode %s", sigData.SignMode.String())
		}

		signBytes, err := pqcsigning.ComputeSignBytes(ctx.ChainID(), account.GetAccountNumber(), txRaw)
		if err != nil {
			return ctx, err
		}

		if pqcDebugEnabled() {
			pqcDebugLogSigner(txHashBytes, signer, "verifying", "")
		}

		if len(entry.Signature) != scheme.SignatureSize() {
			return ctx, errorsmod.Wrapf(pqctypes.ErrPQCVerifyFailed, "signer %s signature length mismatch", signer.String())
		}

		if !scheme.Verify(entry.PubKey, signBytes, entry.Signature) {
			if pqcDebugEnabled() {
				pqcDebugLogSigner(txHashBytes, signer, "verify_failed", "signature_verify_failed")
			}
			if required {
				return ctx, errorsmod.Wrapf(pqctypes.ErrPQCVerifyFailed, "signer %s verification failed", signer.String())
			}
			d.emitEvent(ctx, pqctypes.EventTypeVerifyFailed, signer.String(), map[string]string{
				pqctypes.AttributeKeyReason: "verification_failed",
			})
			continue
		}

		if pqcDebugEnabled() {
			pqcDebugLogSigner(txHashBytes, signer, "verified", "")
		}

		d.emitEvent(ctx, pqctypes.EventTypeVerified, signer.String(), map[string]string{
			pqctypes.AttributeKeyScheme: entry.Scheme,
		})
	}

	return next(ctx, tx, simulate)
}

func (d PQCDualSignDecorator) extractPQCSigs(tx sdk.Tx) (map[string]*pqctypes.PQCSignatureEntry, bool, error) {
	m := make(map[string]*pqctypes.PQCSignatureEntry)

	if provider, ok := tx.(pqcSignatureProvider); ok {
		provided := provider.GetPQCSignatures()
		if len(provided) == 0 {
			return m, false, nil
		}
		for addr, entry := range provided {
			if entry == nil {
				continue
			}
			cloned := *entry
			m[addr] = &cloned
		}
		return m, true, nil
	}

	type extProvider interface {
		GetExtensionOptions() []*codectypes.Any
		GetNonCriticalExtensionOptions() []*codectypes.Any
	}

	provider, ok := tx.(extProvider)
	if !ok {
		return m, false, nil
	}

	for _, any := range append(provider.GetExtensionOptions(), provider.GetNonCriticalExtensionOptions()...) {
		if any == nil || any.TypeUrl != pqctypes.PQCSignaturesTypeURL {
			continue
		}
		var ext pqctypes.PQCSignatures
		if err := d.cdc.Unmarshal(any.Value, &ext); err != nil {
			return nil, false, errorsmod.Wrap(err, "pqc: decode signatures extension")
		}

		for _, sig := range ext.Signatures {
			entry := sig
			m[entry.Addr] = &entry
		}
		return m, true, nil
	}

	return m, false, nil
}

func (d PQCDualSignDecorator) txRawFromContext(ctx sdk.Context) (*txv1beta1.TxRaw, error) {
	txBytes := ctx.TxBytes()
	if len(txBytes) == 0 {
		return nil, fmt.Errorf("pqc: tx bytes unavailable")
	}

	txRaw := new(txv1beta1.TxRaw)
	if err := proto.Unmarshal(txBytes, txRaw); err != nil {
		return nil, errorsmod.Wrap(err, "pqc: decode tx raw")
	}
	return txRaw, nil
}

func computeTxHashBytes(ctx sdk.Context, txRaw *txv1beta1.TxRaw) []byte {
	txBytes := ctx.TxBytes()
	if len(txBytes) == 0 && txRaw != nil {
		if marshaled, err := proto.Marshal(txRaw); err == nil {
			txBytes = marshaled
		}
	}
	if len(txBytes) == 0 {
		return nil
	}
	sum := sha256.Sum256(txBytes)
	return sum[:]
}

func (d PQCDualSignDecorator) emitEvent(ctx sdk.Context, eventType, addr string, attributes map[string]string) {
	attrs := []sdk.Attribute{
		sdk.NewAttribute(pqctypes.AttributeKeyAddress, addr),
	}
	for key, value := range attributes {
		attrs = append(attrs, sdk.NewAttribute(key, value))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(eventType, attrs...))
	if d.logger != nil {
		d.logger.Debug("pqc ante event", "event", eventType, "address", addr, "attrs", attributes)
	}
}

func pendingLinkForSigner(msgs []sdk.Msg, signer sdk.AccAddress) (pqctypes.AccountPQC, bool) {
	signerStr := signer.String()
	for _, msg := range msgs {
		link, ok := msg.(*pqctypes.MsgLinkAccountPQC)
		if !ok {
			continue
		}
		if !strings.EqualFold(link.Creator, signerStr) {
			continue
		}
		hash := sha256.Sum256(link.PubKey)
		account := pqctypes.AccountPQC{
			Addr:       signerStr,
			Scheme:     link.Scheme,
			PubKeyHash: append([]byte(nil), hash[:]...),
		}
		return account, true
	}
	return pqctypes.AccountPQC{}, false
}
