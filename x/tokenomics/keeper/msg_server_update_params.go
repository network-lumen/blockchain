package keeper

import (
	"bytes"
	"context"

	"lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (m msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "request cannot be nil")
	}

	authBz, err := m.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), authBz) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "invalid authority; expected %s, got %s", expected, req.Authority)
	}

	if err := types.ValidateParams(req.Params); err != nil {
		return nil, err
	}

	current := m.GetParams(ctx)
	if err := ensureImmutableParams(current, req.Params); err != nil {
		return nil, err
	}

	if err := m.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

func ensureImmutableParams(current, incoming types.Params) error {
	switch {
	case current.Denom != incoming.Denom:
		return immutableFieldError("denom")
	case current.Decimals != incoming.Decimals:
		return immutableFieldError("decimals")
	case current.SupplyCapLumn != incoming.SupplyCapLumn:
		return immutableFieldError("supply_cap_lumn")
	case current.HalvingIntervalBlocks != incoming.HalvingIntervalBlocks:
		return immutableFieldError("halving_interval_blocks")
	case current.InitialRewardPerBlockLumn != incoming.InitialRewardPerBlockLumn:
		return immutableFieldError("initial_reward_per_block_lumn")
	default:
		return nil
	}
}

func immutableFieldError(field string) error {
	return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "tokenomics: %s is immutable", field)
}
