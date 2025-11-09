package txext

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"

	"google.golang.org/protobuf/proto"

	"lumen/crypto/pqc/dilithium"
	pqckeys "lumen/x/pqc/client/keys"
	pqcsigning "lumen/x/pqc/signing"
	pqctypes "lumen/x/pqc/types"
)

const (
	FlagEnable = "pqc-enable"
	FlagScheme = "pqc-scheme"
	FlagFrom   = "pqc-from"
	FlagKey    = "pqc-key"
)

var pqcDebug = os.Getenv("LUMEN_PQC_DEBUG") == "1"

// InjectPQCPostSign attaches PQC signature data after the standard
// Ed25519/Secp signatures are produced. It returns true if any PQC
// signatures were added and the caller should re-run the standard
// signing routine to cover the updated body bytes.
func InjectPQCPostSign(clientCtx client.Context, builder client.TxBuilder) (bool, error) {
	opts, err := parseOptions(clientCtx)
	if err != nil {
		return false, err
	}
	if !opts.Enabled {
		if err := applyPQCExtension(builder, clientCtx.Codec, nil); err != nil {
			return false, err
		}
		return false, nil
	}

	signingTx := builder.GetTx()
	authTx, ok := signingTx.(authsigning.SigVerifiableTx)
	if !ok {
		return false, fmt.Errorf("pqc: transaction does not expose signer information")
	}

	rawSigners, err := authTx.GetSigners()
	if err != nil {
		return false, fmt.Errorf("pqc: fetch signers: %w", err)
	}
	if len(rawSigners) == 0 {
		return false, nil
	}

	if clientCtx.AccountRetriever == nil {
		return false, fmt.Errorf("pqc: account retriever unavailable")
	}

	scheme := dilithium.Default()
	if !strings.EqualFold(scheme.Name(), opts.Scheme) {
		return false, fmt.Errorf("pqc: active backend %q does not match requested scheme %q", scheme.Name(), opts.Scheme)
	}

	store, err := pqckeys.LoadStore(clientCtx.HomeDir)
	if err != nil {
		return false, err
	}

	params, err := loadParams(clientCtx)
	if err != nil {
		return false, err
	}
	required := params == nil || params.Policy == pqctypes.PqcPolicy_PQC_POLICY_REQUIRED
	if params != nil && params.MinScheme != "" && !strings.EqualFold(params.MinScheme, opts.Scheme) {
		return false, fmt.Errorf("pqc: chain requires min scheme %q (client configured %q)", params.MinScheme, opts.Scheme)
	}

	accountInfoCache := make(map[string]*pqctypes.AccountPQC)

	txBytes, err := clientCtx.TxConfig.TxEncoder()(builder.GetTx())
	if err != nil {
		return false, fmt.Errorf("pqc: encode tx: %w", err)
	}

	txRaw := new(txv1beta1.TxRaw)
	if err := proto.Unmarshal(txBytes, txRaw); err != nil {
		return false, fmt.Errorf("pqc: decode raw tx: %w", err)
	}

	signatureEntries := make([]*pqctypes.PQCSignatureEntry, 0, len(rawSigners))

	for _, signer := range rawSigners {
		addr := sdk.AccAddress(signer)
		addrStr := addr.String()

		accNum, _, err := clientCtx.AccountRetriever.GetAccountNumberSequence(clientCtx, addr)
		if err != nil {
			return false, fmt.Errorf("pqc: fetch account number for %s: %w", addrStr, err)
		}

		keyName := opts.LookupOverride(addrStr, clientCtx.FromAddress)
		if keyName == "" {
			if linked, ok := store.GetLink(addrStr); ok {
				keyName = linked
			}
		}

		keyRecord, hasKey := store.GetKey(keyName)
		if !hasKey {
			if keyName != "" {
				return false, fmt.Errorf("pqc: no local key named %q (required for %s)", keyName, addrStr)
			}
			if pqcDebug {
				fmt.Fprintf(os.Stderr, "[pqc-cli] no pqc key for %s\n", addrStr)
			}
			if required {
				return false, fmt.Errorf("pqc: no local PQC key for %s — import and link a Dilithium key first", addrStr)
			}
			continue
		}

		if !strings.EqualFold(keyRecord.Scheme, opts.Scheme) {
			return false, fmt.Errorf("pqc: local key %s uses scheme %s, expected %s", keyName, keyRecord.Scheme, opts.Scheme)
		}

		if params != nil {
			var info *pqctypes.AccountPQC
			if cached, ok := accountInfoCache[addrStr]; ok {
				info = cached
			} else {
				fetched, err := loadAccount(clientCtx, addrStr)
				if err != nil {
					return false, err
				}
				accountInfoCache[addrStr] = fetched
				info = fetched
			}
			if info != nil && info.Scheme != "" && !strings.EqualFold(info.Scheme, opts.Scheme) {
				return false, fmt.Errorf("pqc: chain registry expects scheme %s for %s (local key %s uses %s)", info.Scheme, addrStr, keyName, opts.Scheme)
			}
		}

		signBytes, err := pqcsigning.ComputeSignBytes(clientCtx.ChainID, accNum, txRaw)
		if err != nil {
			return false, fmt.Errorf("pqc: compute sign bytes: %w", err)
		}

		if pqcDebug {
			digest := sha256.Sum256(signBytes)
			fmt.Fprintf(os.Stderr, "[pqc-cli] signer=%s acc=%d digest=%x\n", addrStr, accNum, digest[:6])
		}

		signature, err := scheme.Sign(keyRecord.PrivateKey, signBytes)
		if err != nil {
			return false, fmt.Errorf("pqc: dilithium sign failed: %w", err)
		}

		if pqcDebug {
			sigHash := sha256.Sum256(signature)
			fmt.Fprintf(os.Stderr, "[pqc-cli] signer=%s sig=%x\n", addrStr, sigHash[:6])
		}

		entry := &pqctypes.PQCSignatureEntry{
			Addr:      addrStr,
			Scheme:    opts.Scheme,
			Signature: signature,
		}
		signatureEntries = append(signatureEntries, entry)
	}

	if len(signatureEntries) == 0 {
		if err := applyPQCExtension(builder, clientCtx.Codec, nil); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := applyPQCExtension(builder, clientCtx.Codec, signatureEntries); err != nil {
		return false, err
	}

	if pqcDebug {
		fmt.Fprintf(os.Stderr, "[pqc-cli] added %d pqc signatures\n", len(signatureEntries))
	}

	return true, nil
}

func parseOptions(clientCtx client.Context) (pqcOptions, error) {
	opts := pqcOptions{
		Enabled: true,
		Scheme:  pqctypes.SchemeDilithium3,
	}

	var v *viper.Viper = clientCtx.Viper
	if v == nil {
		return opts, nil
	}

	if v.IsSet(FlagEnable) {
		opts.Enabled = v.GetBool(FlagEnable)
	}
	if v.IsSet(FlagScheme) {
		opts.Scheme = v.GetString(FlagScheme)
	}

	fromVals := v.GetStringSlice(FlagFrom)
	keyVals := v.GetStringSlice(FlagKey)

	if len(keyVals) != 0 && len(fromVals) != 0 && len(keyVals) != len(fromVals) {
		return opts, fmt.Errorf("pqc: --%s and --%s must be provided the same number of times", FlagFrom, FlagKey)
	}

	overrides := make(map[string]string)

	for i, addr := range fromVals {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			return opts, fmt.Errorf("pqc: --%s entries cannot be empty", FlagFrom)
		}
		if len(keyVals) <= i {
			return opts, fmt.Errorf("pqc: missing --%s entry for %s", FlagKey, addr)
		}
		keyName := strings.TrimSpace(keyVals[i])
		if keyName == "" {
			return opts, fmt.Errorf("pqc: --%s entries cannot be empty", FlagKey)
		}
		overrides[addr] = keyName
	}

	if len(keyVals) == 1 && len(fromVals) == 0 && clientCtx.FromAddress != nil {
		overrides[clientCtx.FromAddress.String()] = strings.TrimSpace(keyVals[0])
	}

	opts.Overrides = overrides
	return opts, nil
}

type pqcOptions struct {
	Enabled   bool
	Scheme    string
	Overrides map[string]string
}

func (o pqcOptions) LookupOverride(addr string, defaultAddr sdk.AccAddress) string {
	if len(o.Overrides) == 0 {
		return ""
	}
	if key, ok := o.Overrides[addr]; ok {
		return key
	}
	if defaultAddr != nil {
		if key, ok := o.Overrides[defaultAddr.String()]; ok {
			return key
		}
	}
	return ""
}

func loadParams(clientCtx client.Context) (*pqctypes.Params, error) {
	if clientCtx.GRPCClient == nil {
		return nil, nil
	}
	queryClient := pqctypes.NewQueryClient(clientCtx.GRPCClient)
	resp, err := queryClient.Params(context.Background(), &pqctypes.QueryParamsRequest{})
	if err != nil {
		return nil, fmt.Errorf("pqc: query params: %w", err)
	}
	params := resp.Params
	return &params, nil
}

func loadAccount(clientCtx client.Context, addr string) (*pqctypes.AccountPQC, error) {
	if clientCtx.GRPCClient == nil {
		return nil, nil
	}
	queryClient := pqctypes.NewQueryClient(clientCtx.GRPCClient)
	resp, err := queryClient.AccountPQC(context.Background(), &pqctypes.QueryAccountPQCRequest{Addr: addr})
	if err != nil {
		return nil, fmt.Errorf("pqc: query account %s: %w", addr, err)
	}
	if resp.Account.Addr == "" {
		return nil, nil
	}
	info := resp.Account
	return &info, nil
}

func applyPQCExtension(builder client.TxBuilder, cdc codec.Codec, entries []*pqctypes.PQCSignatureEntry) error {
	type ncGetter interface {
		GetNonCriticalExtensionOptions() []*codectypes.Any
	}

	var existing []*codectypes.Any
	if getter, ok := builder.(ncGetter); ok {
		existing = getter.GetNonCriticalExtensionOptions()
	}

	filtered := make([]*codectypes.Any, 0, len(existing))
	for _, any := range existing {
		if any == nil || any.TypeUrl == pqctypes.PQCSignaturesTypeURL {
			continue
		}
		filtered = append(filtered, any)
	}

	if len(entries) > 0 {
		values := make([]pqctypes.PQCSignatureEntry, 0, len(entries))
		for _, entry := range entries {
			values = append(values, *entry)
		}
		payload := &pqctypes.PQCSignatures{Signatures: values}
		bz, err := cdc.Marshal(payload)
		if err != nil {
			return fmt.Errorf("pqc: marshal extension: %w", err)
		}
		filtered = append(filtered, &codectypes.Any{
			TypeUrl: pqctypes.PQCSignaturesTypeURL,
			Value:   bz,
		})
	}

	if setter, ok := builder.(interface{ SetNonCriticalExtensionOptions(...*codectypes.Any) }); ok {
		setter.SetNonCriticalExtensionOptions(filtered...)
		return nil
	}

	if setter, ok := builder.(client.ExtendedTxBuilder); ok {
		setter.SetExtensionOptions(filtered...)
		return nil
	}

	if len(entries) == 0 {
		return nil
	}

	return fmt.Errorf("pqc: tx builder does not support extension options")
}
