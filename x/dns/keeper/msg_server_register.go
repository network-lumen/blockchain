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

func (k msgServer) Register(ctx context.Context, msg *types.MsgRegister) (*types.MsgRegisterResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	owner := msg.Owner
	if owner == "" {
		owner = msg.Creator
	}
	if _, err := k.addressCodec.StringToBytes(owner); err != nil {
		return nil, errorsmod.Wrap(err, "invalid owner address")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if k.ak != nil {
		ownerBz, err := k.addressCodec.StringToBytes(owner)
		if err == nil {
			accAddr := sdk.AccAddress(ownerBz)
			if k.ak.GetAccount(ctx, accAddr) == nil {
				newAcc := k.ak.NewAccountWithAddress(ctx, accAddr)
				k.ak.SetAccount(ctx, newAcc)
				sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
					"dns_owner_account_created",
					sdk.NewAttribute("owner", owner),
					sdk.NewAttribute("reason", "dns_register"),
				))
			}
		}
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

	now := k.nowSec(ctx)
	cur, err := k.Domain.Get(ctx, name)
	found := err == nil
	if found {
		status := lifecycleStatus(now, cur.ExpireAt, params.GraceDays, params.AuctionDays)
		if status == "active" || status == "grace" || status == "auction" {
			return nil, types.ErrDomainExists
		}
	}

	days := defaultDays(msg.DurationDays, types.MaxRegistrationDurationDays)
	if days > types.MaxRegistrationDurationDays {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("duration_days cannot exceed %d", types.MaxRegistrationDurationDays)
	}
	expire := now + days*24*3600

	priceDec, priceInt, err := params.PriceQuote(len(domain), len(ext), days)
	if err != nil {
		return nil, err
	}

	if err := types.ValidateRecords(msg.Records); err != nil {
		return nil, err
	}

	charge := true

	if charge && priceInt.IsPositive() && k.bank != nil {
		creatorBz, _ := k.addressCodec.StringToBytes(msg.Creator)
		payerAddr := sdk.AccAddress(creatorBz)
		coin := sdk.NewCoin(denom.BaseDenom, priceInt)
		if err := k.bank.SendCoinsFromAccountToModule(ctx, payerAddr, authtypes.FeeCollectorName, sdk.NewCoins(coin)); err != nil {
			return nil, err
		}
	}

	newDom := types.Domain{
		Index:     name,
		Name:      name,
		Owner:     owner,
		Records:   msg.Records,
		ExpireAt:  expire,
		Creator:   msg.Creator,
		UpdatedAt: now,
	}
	if err := k.Domain.Set(ctx, name, newDom); err != nil {
		return nil, err
	}

	cnt, _ := k.OpsThisBlock.Get(ctx)
	_ = k.OpsThisBlock.Set(ctx, cnt+1)

	chargedStr := "false"
	amountStr := "0"
	if charge {
		chargedStr = "true"
		if priceInt.IsPositive() {
			amountStr = priceInt.String()
		}
	}
	attrs := []sdk.Attribute{
		sdk.NewAttribute("name", name),
		sdk.NewAttribute("owner", owner),
		sdk.NewAttribute("created_by", msg.Creator),
		sdk.NewAttribute("charged", chargedStr),
		sdk.NewAttribute("amount_paid_ulmn", amountStr),
		sdk.NewAttribute("price_estimate_dec", priceDec.String()),
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent("dns_register", attrs...))

	return &types.MsgRegisterResponse{}, nil
}
