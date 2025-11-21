package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/x/dns/types"
)

func (k msgServer) Update(ctx context.Context, msg *types.MsgUpdate) (*types.MsgUpdateResponse, error) {
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
	now := k.nowSec(ctx)
	if err := enforceUpdateRateLimit(dom.UpdatedAt, now, params.UpdateRateLimitSeconds); err != nil {
		return nil, err
	}
	if err := enforceUpdatePoW(name, msg.Creator, msg.PowNonce, params.UpdatePowDifficulty); err != nil {
		return nil, err
	}

	if len(msg.Records) > 0 {
		if err := types.ValidateRecords(msg.Records); err != nil {
			return nil, err
		}
		dom.Records = msg.Records
	}
	dom.UpdatedAt = now

	if err := k.Domain.Set(ctx, name, dom); err != nil {
		return nil, err
	}

	// no fixed fee for updates

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(
		sdk.NewEvent("dns_update", sdk.NewAttribute("name", name)),
	)
	return &types.MsgUpdateResponse{}, nil
}
