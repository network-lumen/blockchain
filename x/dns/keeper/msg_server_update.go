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

func (k msgServer) Update(ctx context.Context, msg *types.MsgUpdate) (*types.MsgUpdateResponse, error) {
	creatorBz, err := k.addressCodec.StringToBytes(msg.Creator)
	if err != nil {
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
	now := k.nowSec(ctx)
	if err := enforceUpdateRateLimit(dom.UpdatedAt, now, params.UpdateRateLimitSeconds); err != nil {
		return nil, err
	}
	if err := enforceUpdatePoW(name, msg.Creator, msg.PowNonce, params.UpdatePowDifficulty); err != nil {
		return nil, err
	}
	feeInt := sdkmath.NewIntFromUint64(params.UpdateFeeUlmn)

	if len(msg.Records) > 0 {
		if err := types.ValidateRecords(msg.Records); err != nil {
			return nil, err
		}
		dom.Records = msg.Records
	}
	dom.UpdatedAt = now

	if feeInt.IsPositive() {
		if k.bank == nil {
			return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "bank keeper unavailable while update_fee_ulmn > 0")
		}
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		from := sdk.AccAddress(creatorBz)
		feeCoin := sdk.NewCoin(denom.BaseDenom, feeInt)
		if err := k.bank.SendCoinsFromAccountToModule(sdkCtx, from, authtypes.FeeCollectorName, sdk.NewCoins(feeCoin)); err != nil {
			return nil, err
		}
	}

	if err := k.Domain.Set(ctx, name, dom); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(
		sdk.NewEvent(
			"dns_update",
			sdk.NewAttribute("name", name),
			sdk.NewAttribute("fee_ulmn", feeInt.String()),
		),
	)
	return &types.MsgUpdateResponse{}, nil
}
