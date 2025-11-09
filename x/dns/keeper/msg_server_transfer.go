package keeper

import (
	"context"

	"lumen/app/denom"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"lumen/x/dns/types"
)

func (k msgServer) Transfer(ctx context.Context, msg *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}
	if _, err := k.addressCodec.StringToBytes(msg.NewOwner); err != nil {
		return nil, errorsmod.Wrap(err, "invalid new_owner address")
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

	feeInt := sdkmath.NewIntFromUint64(params.TransferFeeUlmn)
	if feeInt.IsPositive() && k.bank != nil {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		creatorBz, _ := k.addressCodec.StringToBytes(msg.Creator)
		from := sdk.AccAddress(creatorBz)
		feeCoin := sdk.NewCoin(denom.BaseDenom, feeInt)
		if err := k.bank.SendCoinsFromAccountToModule(sdkCtx, from, authtypes.FeeCollectorName, sdk.NewCoins(feeCoin)); err != nil {
			return nil, err
		}
	}

	dom.Owner = msg.NewOwner
	if err := k.Domain.Set(ctx, name, dom); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(
		sdk.NewEvent("dns_transfer",
			sdk.NewAttribute("name", name),
			sdk.NewAttribute("from", msg.Creator),
			sdk.NewAttribute("to", msg.NewOwner),
			sdk.NewAttribute("fee_ulmn", feeInt.String()),
		),
	)
	return &types.MsgTransferResponse{}, nil
}
