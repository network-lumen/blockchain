package keeper

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"lumen/x/release/types"
)

type msgServer struct {
	Keeper
}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

func (m msgServer) RejectRelease(ctx context.Context, msg *types.MsgRejectRelease) (*types.MsgRejectReleaseResponse, error) {
	if err := m.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}
	release, err := m.Release.Get(ctx, msg.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(sdkerrors.ErrKeyNotFound, "release %d not found", msg.Id)
		}
		return nil, err
	}
	if err := m.enforcePendingAndNotYanked(release); err != nil {
		return nil, err
	}

	release.Status = types.Release_REJECTED
	release.EmergencyOk = false
	release.EmergencyUntil = 0
	if err := m.Release.Set(ctx, release.Id, release); err != nil {
		return nil, err
	}
	if err := m.dequeueExpiry(ctx, release.Id); err != nil {
		return nil, err
	}
	if err := m.forfeitEscrowToCommunityPool(ctx, release.Id); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"release_reject",
		sdk.NewAttribute("id", fmt.Sprintf("%d", release.Id)),
		sdk.NewAttribute("status", release.Status.String()),
	))

	return &types.MsgRejectReleaseResponse{}, nil
}

func (m msgServer) SetEmergency(ctx context.Context, msg *types.MsgSetEmergency) (*types.MsgSetEmergencyResponse, error) {
	return nil, errorsmod.Wrap(types.ErrDisabled, "emergency rollout is disabled")
}

func (m msgServer) ValidateRelease(ctx context.Context, msg *types.MsgValidateRelease) (*types.MsgValidateReleaseResponse, error) {
	if err := m.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}
	release, err := m.Release.Get(ctx, msg.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(sdkerrors.ErrKeyNotFound, "release %d not found", msg.Id)
		}
		return nil, err
	}
	if err := m.enforcePendingAndNotYanked(release); err != nil {
		return nil, err
	}

	release.Status = types.Release_VALIDATED
	release.EmergencyOk = false
	release.EmergencyUntil = 0
	if err := m.Release.Set(ctx, release.Id, release); err != nil {
		return nil, err
	}
	if err := m.dequeueExpiry(ctx, release.Id); err != nil {
		return nil, err
	}
	if err := m.refundEscrow(ctx, release.Id); err != nil {
		return nil, err
	}

	for _, a := range release.Artifacts {
		if a == nil {
			continue
		}
		key := tripleKey(release.Channel, a.Platform, a.Kind)
		existingID, err := m.ByTriple.Get(ctx, key)
		if err == nil && existingID > release.Id {
			continue
		}
		if err := m.ByTriple.Set(ctx, key, release.Id); err != nil {
			return nil, err
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"release_validate",
		sdk.NewAttribute("id", fmt.Sprintf("%d", release.Id)),
		sdk.NewAttribute("status", release.Status.String()),
	))

	return &types.MsgValidateReleaseResponse{}, nil
}

func (m msgServer) assertAuthority(authority string) error {
	addr, err := m.addressCodec.StringToBytes(authority)
	if err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(m.GetAuthority(), addr) {
		expected, _ := m.addressCodec.BytesToString(m.GetAuthority())
		return errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, authority)
	}
	return nil
}
