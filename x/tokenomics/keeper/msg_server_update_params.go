package keeper

import (
	"bytes"
	"context"

	"lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
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
	if err := m.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
