package keeper

import (
	"context"

	"lumen/app/denom"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"lumen/x/dns/types"
)

func (k msgServer) Bid(ctx context.Context, msg *types.MsgBid) (*types.MsgBidResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	domain := types.NormalizeDomain(msg.Domain)
	ext := types.NormalizeExt(msg.Ext)
	if err := types.ValidateDomainParts(domain, ext); err != nil {
		return nil, err
	}
	name := k.fqdn(domain, ext)

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	dom, err := k.Domain.Get(ctx, name)
	if err != nil {
		return nil, types.ErrInvalidFqdn
	}

	now := k.nowSec(ctx)
	status := lifecycleStatus(now, dom.ExpireAt, params.GraceDays, params.AuctionDays)
	if status != "auction" {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "auction not open")
	}

	auc, err := k.Auction.Get(ctx, name)
	if err != nil {
		start := dom.ExpireAt + params.GraceDays*24*3600
		end := start + params.AuctionDays*24*3600
		auc = types.Auction{
			Creator:    msg.Creator,
			Index:      name,
			Name:       name,
			Start:      start,
			End:        end,
			HighestBid: "",
			Bidder:     "",
		}
	}

	bidAmt, ok := sdkmath.NewIntFromString(msg.Amount)
	if !ok || !bidAmt.IsPositive() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid bid amount")
	}
	_, minBid, err := params.PriceQuote(len(domain), len(ext), defaultDays(0, 365))
	if err != nil {
		return nil, err
	}
	if bidAmt.LT(minBid) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "bid must be >= %s", minBid.String())
	}

	curAmt := sdkmath.ZeroInt()
	if auc.HighestBid != "" {
		if v, ok2 := sdkmath.NewIntFromString(auc.HighestBid); ok2 {
			curAmt = v
		}
	}

	if !bidAmt.GT(curAmt) {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "insufficient bid")
	}

	auc.HighestBid = bidAmt.String()
	auc.Bidder = msg.Creator

	if err := k.Auction.Set(ctx, name, auc); err != nil {
		return nil, err
	}

	if k.bank != nil {
		fee := sdkmath.NewIntFromUint64(params.BidFeeUlmn)
		if fee.IsPositive() {
			fromBz, _ := k.addressCodec.StringToBytes(msg.Creator)
			from := sdk.AccAddress(fromBz)
			coins := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, fee))
			if err := k.bank.SendCoinsFromAccountToModule(sdk.UnwrapSDKContext(ctx), from, authtypes.FeeCollectorName, coins); err != nil {
				return nil, err
			}
		}
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(
		sdk.NewEvent("dns_bid",
			sdk.NewAttribute("name", name),
			sdk.NewAttribute("bidder", msg.Creator),
			sdk.NewAttribute("amount", bidAmt.String()),
		),
	)

	return &types.MsgBidResponse{}, nil
}
