package keeper

import (
	"bytes"
	"context"

	"lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (m msgServer) CommunityPoolSpend(ctx context.Context, req *types.MsgCommunityPoolSpend) (*types.MsgCommunityPoolSpendResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "request cannot be nil")
	}

	if !m.HasDistributionKeeper() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "distribution keeper is not configured")
	}

	authBz, err := m.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), authBz) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "invalid authority; expected %s, got %s", expected, req.Authority)
	}

	if req.Recipient == "" {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "recipient must be set")
	}
	recipient, err := m.addressCodec.StringToBytes(req.Recipient)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid recipient address")
	}

	amount := sdk.NewCoins(req.Amount...)
	if amount.Empty() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "amount must be set")
	}
	if !amount.IsValid() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "amount is invalid")
	}
	if !amount.IsAllPositive() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "amount must be > 0")
	}

	if err := m.distr.CommunityPoolSpend(ctx, recipient, amount); err != nil {
		return nil, err
	}

	return &types.MsgCommunityPoolSpendResponse{}, nil
}
