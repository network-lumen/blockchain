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

func (k msgServer) Settle(ctx context.Context, msg *types.MsgSettle) (*types.MsgSettleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid creator")
	}

	domain := types.NormalizeDomain(msg.Domain)
	ext := types.NormalizeExt(msg.Ext)
	if err := types.ValidateDomainParts(domain, ext); err != nil {
		return nil, err
	}
	name := k.fqdn(domain, ext)

	dom, err := k.Domain.Get(ctx, name)
	if err != nil {
		return nil, sdkerrors.ErrNotFound.Wrap("domain not found")
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	now := k.nowSec(ctx)
	graceEnd := dom.ExpireAt + params.GraceDays*24*3600
	auctionEnd := graceEnd + params.AuctionDays*24*3600
	if now < auctionEnd {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("auction not finished yet")
	}

	auc, err := k.Auction.Get(ctx, name)
	if err != nil {
		return nil, sdkerrors.ErrNotFound.Wrap("auction not found")
	}
	if auc.Bidder == "" || auc.HighestBid == "" {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("no winner to settle")
	}

	dom.Owner = auc.Bidder
	if err := k.Domain.Set(ctx, name, dom); err != nil {
		return nil, err
	}

	if k.bank != nil && auc.HighestBid != "" {
		if amt, ok := sdkmath.NewIntFromString(auc.HighestBid); ok && amt.IsPositive() {
			winnerBz, _ := k.addressCodec.StringToBytes(auc.Bidder)
			winner := sdk.AccAddress(winnerBz)

			bps := int64(100)
			proposer := amt.MulRaw(bps).QuoRaw(10000)
			if proposer.IsZero() && amt.IsPositive() && bps > 0 {
				proposer = sdkmath.NewInt(1)
			}
			if proposer.GT(amt) {
				proposer = amt
			}
			network := amt.Sub(proposer)

			if err := k.bank.SendCoinsFromAccountToModule(ctx, winner, types.ModuleName, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, amt))); err != nil {
				return nil, err
			}

			moduleAddr := authtypes.NewModuleAddress(types.ModuleName)

			if proposer.IsPositive() {
				propCoins := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, proposer))
				if k.dk != nil {
					if err := k.dk.FundCommunityPool(ctx, propCoins, moduleAddr); err != nil {
						return nil, err
					}
				} else if err := k.bank.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, propCoins); err != nil {
					return nil, err
				}
			}

			if network.IsPositive() {
				netCoins := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, network))
				if k.dk != nil {
					if err := k.dk.FundCommunityPool(ctx, netCoins, moduleAddr); err != nil {
						return nil, err
					}
				} else if err := k.bank.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, netCoins); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := k.Auction.Remove(ctx, name); err != nil {
		return nil, err
	}

	var propStr, netStr string
	if amt, ok := sdkmath.NewIntFromString(auc.HighestBid); ok {
		bps := int64(100)
		ps := amt.MulRaw(bps).QuoRaw(10000)
		if ps.IsZero() && amt.IsPositive() && bps > 0 {
			ps = sdkmath.NewInt(1)
		}
		if ps.GT(amt) {
			ps = amt
		}
		ns := amt.Sub(ps)
		propStr = ps.String()
		netStr = ns.String()
	}
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("dns_settle",
			sdk.NewAttribute("name", name),
			sdk.NewAttribute("winner", auc.Bidder),
			sdk.NewAttribute("amount_ulmn", auc.HighestBid),
			sdk.NewAttribute("proposer_share_ulmn", propStr),
			sdk.NewAttribute("network_share_ulmn", netStr),
		),
	)

	return &types.MsgSettleResponse{}, nil
}
