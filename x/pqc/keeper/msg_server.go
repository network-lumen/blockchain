package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

	existing, found, err := m.keeper.GetAccountPQC(goCtx, creatorAddr)
	if err != nil {
		return nil, err
	}
	if found && !params.AllowAccountRotate {
		return nil, types.ErrAccountRotationDisabled
	}

	account := types.AccountPQC{
		Addr:    creatorAddr.String(),
		Scheme:  msg.Scheme,
		PubKey:  append([]byte(nil), msg.PubKey...),
		AddedAt: sdkCtx.BlockTime().Unix(),
	}

	if err := m.keeper.SetAccountPQC(goCtx, creatorAddr, account); err != nil {
		return nil, err
	}

	if !found {
		m.emitLinkEvent(sdkCtx, creatorAddr.String(), msg.Scheme, msg.PubKey)
	} else if params.AllowAccountRotate &&
		!equalAccount(existing, account) {
		m.emitLinkEvent(sdkCtx, creatorAddr.String(), msg.Scheme, msg.PubKey)
	}

	return &types.MsgLinkAccountPQCResponse{}, nil
}

func (m msgServer) emitLinkEvent(ctx sdk.Context, addr, scheme string, pubKey []byte) {
	hash := sha256.Sum256(pubKey)
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeLinkAccount,
			sdk.NewAttribute(types.AttributeKeyAddress, addr),
			sdk.NewAttribute(types.AttributeKeyScheme, scheme),
			sdk.NewAttribute(types.AttributeKeyPubKeyHash, hex.EncodeToString(hash[:])),
		),
	)
}

func equalAccount(a, b types.AccountPQC) bool {
	if !strings.EqualFold(a.Scheme, b.Scheme) {
		return false
	}
	if len(a.PubKey) != len(b.PubKey) {
		return false
	}
	for i := range a.PubKey {
		if a.PubKey[i] != b.PubKey[i] {
			return false
		}
	}
	return true
}
