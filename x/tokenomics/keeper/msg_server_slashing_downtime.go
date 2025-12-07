package keeper

import (
	"bytes"
	"context"
	"time"

	"lumen/x/tokenomics/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	minDowntimeJailSeconds int64 = 60           // 60s
	maxDowntimeJailSeconds int64 = 48 * 60 * 60 // 48h
)

func (m msgServer) UpdateSlashingDowntimeParams(ctx context.Context, req *types.MsgUpdateSlashingDowntimeParams) (*types.MsgUpdateSlashingDowntimeParamsResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(types.ErrUnauthorized, "request cannot be nil")
	}

	if !m.HasSlashingKeeper() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "slashing keeper is not configured")
	}

	authBz, err := m.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), authBz) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "invalid authority; expected %s, got %s", expected, req.Authority)
	}

	if req.SlashFractionDowntime == "" {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "slash_fraction_downtime must be set")
	}
	if req.DowntimeJailDuration == "" {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "downtime_jail_duration must be set")
	}

	frac, err := sdkmath.LegacyNewDecFromStr(req.SlashFractionDowntime)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid slash_fraction_downtime: %v", err)
	}
	if frac.IsNegative() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "slash_fraction_downtime must be >= 0")
	}
	maxFrac := sdkmath.LegacyNewDecWithPrec(5, 2) // 0.05
	if frac.GT(maxFrac) {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "slash_fraction_downtime must be <= 0.05")
	}

	dur, err := time.ParseDuration(req.DowntimeJailDuration)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid downtime_jail_duration: %v", err)
	}
	seconds := int64(dur.Seconds())
	if seconds < minDowntimeJailSeconds {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "downtime_jail_duration must be >= %ds", minDowntimeJailSeconds)
	}
	if seconds > maxDowntimeJailSeconds {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "downtime_jail_duration must be <= %ds", maxDowntimeJailSeconds)
	}

	params, err := m.slashing.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	params.SlashFractionDowntime = frac
	params.DowntimeJailDuration = dur

	if err := m.slashing.SetParams(ctx, params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateSlashingDowntimeParamsResponse{}, nil
}
