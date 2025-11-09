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

func (k msgServer) CreateAuction(ctx context.Context, msg *types.MsgCreateAuction) (*types.MsgCreateAuctionResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, fmt.Sprintf("invalid address: %s", err))
	}

	ok, err := k.Auction.Has(ctx, msg.Index)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	} else if ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "index already set")
	}

	var auction = types.Auction{
		Creator:    msg.Creator,
		Index:      msg.Index,
		Name:       msg.Name,
		Start:      msg.Start,
		End:        msg.End,
		HighestBid: msg.HighestBid,
		Bidder:     msg.Bidder,
	}

	if err := k.Auction.Set(ctx, auction.Index, auction); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}

	return &types.MsgCreateAuctionResponse{}, nil
}

func (k msgServer) UpdateAuction(ctx context.Context, msg *types.MsgUpdateAuction) (*types.MsgUpdateAuctionResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, fmt.Sprintf("invalid signer address: %s", err))
	}

	val, err := k.Auction.Get(ctx, msg.Index)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrap(sdkerrors.ErrKeyNotFound, "index not set")
		}

		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}

	if msg.Creator != val.Creator {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "incorrect owner")
	}

	var auction = types.Auction{
		Creator:    msg.Creator,
		Index:      msg.Index,
		Name:       msg.Name,
		Start:      msg.Start,
		End:        msg.End,
		HighestBid: msg.HighestBid,
		Bidder:     msg.Bidder,
	}

	if err := k.Auction.Set(ctx, auction.Index, auction); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "failed to update auction")
	}

	return &types.MsgUpdateAuctionResponse{}, nil
}

func (k msgServer) DeleteAuction(ctx context.Context, msg *types.MsgDeleteAuction) (*types.MsgDeleteAuctionResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, fmt.Sprintf("invalid signer address: %s", err))
	}

	val, err := k.Auction.Get(ctx, msg.Index)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrap(sdkerrors.ErrKeyNotFound, "index not set")
		}

		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}

	if msg.Creator != val.Creator {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "incorrect owner")
	}

	if err := k.Auction.Remove(ctx, msg.Index); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "failed to remove auction")
	}

	return &types.MsgDeleteAuctionResponse{}, nil
}
