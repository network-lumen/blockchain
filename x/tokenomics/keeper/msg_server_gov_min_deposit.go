package keeper

import (
	"bytes"
	"context"

	"lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var maxGovMinDepositUlmn = sdkmath.NewInt(1_000_000_000_000)

func (m msgServer) UpdateGovMinDeposit(ctx context.Context, req *types.MsgUpdateGovMinDeposit) (*types.MsgUpdateGovMinDepositResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "request cannot be nil")
	}

	if !m.HasGovKeeper() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "gov keeper is not configured")
	}

	authBz, err := m.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), authBz) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "invalid authority; expected %s, got %s", expected, req.Authority)
	}

	minDeposit := sdk.NewCoins(req.MinDeposit...)
	if minDeposit.Empty() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_deposit must be set")
	}
	if !minDeposit.IsValid() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_deposit is invalid")
	}
	if len(minDeposit) != 1 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_deposit must contain exactly one coin")
	}
	denom := m.GetParams(ctx).Denom
	coin := minDeposit[0]
	if coin.Denom != denom {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "min_deposit denom must be %s", denom)
	}
	if !coin.Amount.IsPositive() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "min_deposit must be > 0")
	}
	if coin.Amount.GT(maxGovMinDepositUlmn) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "min_deposit must be <= %s%s", maxGovMinDepositUlmn.String(), denom)
	}

	params, err := m.gov.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	params.MinDeposit = minDeposit
	if err := params.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := m.gov.SetParams(ctx, params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateGovMinDepositResponse{}, nil
}
