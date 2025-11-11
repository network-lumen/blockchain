package keeper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"lumen/x/pqc/types"
)

type msgServer struct {
	keeper Keeper
}

func NewMsgServerImpl(k Keeper) types.MsgServer {
	return &msgServer{keeper: k}
}

func (m msgServer) LinkAccountPQC(goCtx context.Context, msg *types.MsgLinkAccountPQC) (*types.MsgLinkAccountPQCResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(goCtx)
	params := m.keeper.GetParams(goCtx)

	if !types.IsSupportedScheme(msg.Scheme) {
		return nil, types.ErrSchemeUnsupported
	}
	if !strings.EqualFold(msg.Scheme, params.MinScheme) {
		return nil, errorsmod.Wrapf(types.ErrSchemeUnsupported, "min scheme requirement %s", params.MinScheme)
	}

	creatorAddr, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "creator: %v", err)
	}

	scheme := m.keeper.Scheme()
	if !strings.EqualFold(scheme.Name(), msg.Scheme) {
		return nil, errorsmod.Wrapf(types.ErrSchemeUnsupported, "active backend %s", scheme.Name())
	}
	if len(msg.PubKey) != scheme.PublicKeySize() {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidPubKey, "expected %d bytes", scheme.PublicKeySize())
	}

	if err := m.ensureMinBalance(sdkCtx, creatorAddr, params); err != nil {
		return nil, err
	}
	if err := validatePow(msg.PubKey, msg.PowNonce, params.PowDifficultyBits); err != nil {
		return nil, err
	}

	existing, found, err := m.keeper.GetAccountPQC(goCtx, creatorAddr)
	if err != nil {
		return nil, err
	}
	if found && !params.AllowAccountRotate {
		return nil, types.ErrAccountRotationDisabled
	}

	hash := sha256.Sum256(msg.PubKey)
	pubKeyHash := append([]byte(nil), hash[:]...)

	account := types.AccountPQC{
		Addr:       creatorAddr.String(),
		Scheme:     msg.Scheme,
		PubKeyHash: pubKeyHash,
		AddedAt:    sdkCtx.BlockTime().Unix(),
	}

	if err := m.keeper.SetAccountPQC(goCtx, creatorAddr, account); err != nil {
		return nil, err
	}

	if !found {
		m.emitLinkEvent(sdkCtx, creatorAddr.String(), msg.Scheme, pubKeyHash, params)
	} else if params.AllowAccountRotate &&
		!equalAccount(existing, account) {
		m.emitLinkEvent(sdkCtx, creatorAddr.String(), msg.Scheme, pubKeyHash, params)
	}

	return &types.MsgLinkAccountPQCResponse{}, nil
}

func (m msgServer) ensureMinBalance(ctx sdk.Context, addr sdk.AccAddress, params types.Params) error {
	if params.MinBalanceForLink.IsNil() || !params.MinBalanceForLink.IsPositive() {
		return nil
	}
	if m.keeper.BankKeeper() == nil {
		return fmt.Errorf("bank keeper not configured for pqc module")
	}
	spendable := m.keeper.BankKeeper().SpendableCoins(ctx, addr)
	if !spendable.AmountOf(params.MinBalanceForLink.Denom).GTE(params.MinBalanceForLink.Amount) {
		return errorsmod.Wrapf(types.ErrInsufficientBalanceLink, "requires %s", params.MinBalanceForLink.String())
	}
	return nil
}

func validatePow(pubKey, nonce []byte, bits uint32) error {
	if bits == 0 {
		return nil
	}
	sum := types.ComputePowDigest(pubKey, nonce)
	if leading := types.LeadingZeroBits(sum[:]); leading < int(bits) {
		return errorsmod.Wrapf(types.ErrInvalidPow, "need %d leading zero bits, got %d", bits, leading)
	}
	return nil
}

func (m msgServer) emitLinkEvent(ctx sdk.Context, addr, scheme string, pubKeyHash []byte, params types.Params) {
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeLinkAccount,
			sdk.NewAttribute(types.AttributeKeyAddress, addr),
			sdk.NewAttribute(types.AttributeKeyScheme, scheme),
			sdk.NewAttribute(types.AttributeKeyPubKeyHash, hex.EncodeToString(pubKeyHash)),
			sdk.NewAttribute(types.AttributeKeyPowDifficulty, strconv.FormatUint(uint64(params.PowDifficultyBits), 10)),
			sdk.NewAttribute(types.AttributeKeyMinBalanceUsed, params.MinBalanceForLink.String()),
		),
	)
}

func equalAccount(a, b types.AccountPQC) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && bytes.Equal(a.PubKeyHash, b.PubKeyHash)
}
