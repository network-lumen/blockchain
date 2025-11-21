package keeper

import (
	"context"

	"lumen/app/denom"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"lumen/x/dns/types"
)

func (k msgServer) Renew(ctx context.Context, msg *types.MsgRenew) (*types.MsgRenewResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	domain := types.NormalizeDomain(msg.Domain)
	ext := types.NormalizeExt(msg.Ext)
	if err := types.ValidateDomainParts(domain, ext); err != nil {
		return nil, err
	}
	name := k.fqdn(domain, ext)

	dom, err := k.Domain.Get(ctx, name)
	if err != nil {
		return nil, types.ErrInvalidFqdn
	}
	if dom.Owner != msg.Creator {
		return nil, types.ErrNotOwner
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	days := defaultDays(msg.DurationDays, types.MaxRegistrationDurationDays)
	if days > types.MaxRegistrationDurationDays {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("duration_days cannot exceed %d", types.MaxRegistrationDurationDays)
	}
	priceDec, priceInt, err := params.PriceQuote(len(domain), len(ext), days)
	if err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if priceInt.IsPositive() {
		addrBz, _ := k.addressCodec.StringToBytes(msg.Creator)
		acc := sdk.AccAddress(addrBz)
		coin := sdk.NewCoin(denom.BaseDenom, priceInt)
		coins := sdk.NewCoins(coin)
		switch {
		case k.dk != nil:
			if err := k.dk.FundCommunityPool(ctx, coins, acc); err != nil {
				return nil, err
			}
		case k.bank != nil:
			if err := k.bank.SendCoinsFromAccountToModule(sdkCtx, acc, authtypes.FeeCollectorName, coins); err != nil {
				return nil, err
			}
		default:
			return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "bank and distribution keepers unavailable")
		}
	}

	sec := days * 24 * 3600
	now := k.nowSec(ctx)
	if dom.ExpireAt == 0 {
		dom.ExpireAt = now
	}
	dom.ExpireAt += sec

	if err := k.Domain.Set(ctx, name, dom); err != nil {
		return nil, err
	}

	cnt, _ := k.OpsThisBlock.Get(ctx)
	_ = k.OpsThisBlock.Set(ctx, cnt+1)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("dns_renew",
			sdk.NewAttribute("name", name),
			sdk.NewAttribute("amount_paid_ulmn", priceInt.String()),
			sdk.NewAttribute("price_dec", priceDec.String()),
		),
	)
	return &types.MsgRenewResponse{}, nil
}
