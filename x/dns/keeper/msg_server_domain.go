package keeper

import (
	"context"
	"errors"
	"fmt"

	"lumen/x/dns/types"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (k msgServer) CreateDomain(ctx context.Context, msg *types.MsgCreateDomain) (*types.MsgCreateDomainResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, fmt.Sprintf("invalid address: %s", err))
	}

	ok, err := k.Domain.Has(ctx, msg.Index)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	} else if ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "index already set")
	}

	if err := types.ValidateRecords(msg.Records); err != nil {
		return nil, err
	}

	var domain = types.Domain{
		Creator:   msg.Creator,
		Index:     msg.Index,
		Name:      msg.Name,
		Owner:     msg.Owner,
		Records:   msg.Records,
		ExpireAt:  msg.ExpireAt,
		UpdatedAt: k.nowSec(ctx),
	}

	if err := k.Domain.Set(ctx, domain.Index, domain); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}

	return &types.MsgCreateDomainResponse{}, nil
}

func (k msgServer) UpdateDomain(ctx context.Context, msg *types.MsgUpdateDomain) (*types.MsgUpdateDomainResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, fmt.Sprintf("invalid signer address: %s", err))
	}

	val, err := k.Domain.Get(ctx, msg.Index)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrap(sdkerrors.ErrKeyNotFound, "index not set")
		}
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}

	if msg.Creator != val.Creator {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "incorrect owner")
	}

	if err := types.ValidateRecords(msg.Records); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	now := k.nowSec(ctx)
	if err := enforceUpdateRateLimit(val.UpdatedAt, now, params.UpdateRateLimitSeconds); err != nil {
		return nil, err
	}
	if err := enforceUpdatePoW(msg.Index, msg.Creator, msg.PowNonce, params.UpdatePowDifficulty); err != nil {
		return nil, err
	}

	var updated = types.Domain{
		Creator:   msg.Creator,
		Index:     msg.Index,
		Name:      msg.Name,
		Owner:     msg.Owner,
		Records:   msg.Records,
		ExpireAt:  msg.ExpireAt,
		UpdatedAt: now,
	}

	if err := k.Domain.Set(ctx, updated.Index, updated); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "failed to update domain")
	}

	return &types.MsgUpdateDomainResponse{}, nil
}

func (k msgServer) DeleteDomain(ctx context.Context, msg *types.MsgDeleteDomain) (*types.MsgDeleteDomainResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, fmt.Sprintf("invalid signer address: %s", err))
	}

	val, err := k.Domain.Get(ctx, msg.Index)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrap(sdkerrors.ErrKeyNotFound, "index not set")
		}
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}

	if msg.Creator != val.Creator {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "incorrect owner")
	}

	if err := k.Domain.Remove(ctx, msg.Index); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "failed to remove domain")
	}

	return &types.MsgDeleteDomainResponse{}, nil
}
