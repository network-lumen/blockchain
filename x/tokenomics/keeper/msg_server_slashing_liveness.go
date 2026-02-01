package keeper

import (
	"bytes"
	"context"

	"lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (m msgServer) UpdateSlashingLivenessParams(ctx context.Context, req *types.MsgUpdateSlashingLivenessParams) (*types.MsgUpdateSlashingLivenessParamsResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "request cannot be nil")
	}

	if !m.HasSlashingKeeper() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "slashing keeper is not configured")
	}

	authBz, err := m.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), authBz) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "invalid authority; expected %s, got %s", expected, req.Authority)
	}

	if req.SignedBlocksWindow <= 0 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "signed_blocks_window must be > 0")
	}
	if req.MinSignedPerWindow == "" {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_signed_per_window must be set")
	}

	minSignedPerWindow, err := sdkmath.LegacyNewDecFromStr(req.MinSignedPerWindow)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid min_signed_per_window: %v", err)
	}
	if minSignedPerWindow.IsNegative() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_signed_per_window must be >= 0")
	}
	if minSignedPerWindow.GT(sdkmath.LegacyOneDec()) {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_signed_per_window must be <= 1")
	}

	params, err := m.slashing.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	params.SignedBlocksWindow = req.SignedBlocksWindow
	params.MinSignedPerWindow = minSignedPerWindow

	if err := m.slashing.SetParams(ctx, params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateSlashingLivenessParamsResponse{}, nil
}
