package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"

	"lumen/x/dns/types"
)

func (k msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authority, err := k.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}

	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, req.Authority)
	}

	if err := req.Params.Validate(); err != nil {
		return nil, err
	}

	cur, _ := k.Params.Get(ctx)
	if req.Params.BaseFeeDns != cur.BaseFeeDns {
		return nil, errorsmod.Wrapf(types.ErrInvalidRequest, "base_fee_dns is fixed and cannot be changed")
	}
	if err := k.Params.Set(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
