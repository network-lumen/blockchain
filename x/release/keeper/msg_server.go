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

	if err := m.updateReleaseStatus(ctx, msg.Id, types.Release_REJECTED); err != nil {
		return nil, err
	}

	return &types.MsgRejectReleaseResponse{}, nil
}

func (m msgServer) SetEmergency(ctx context.Context, msg *types.MsgSetEmergency) (*types.MsgSetEmergencyResponse, error) {
	if err := m.assertAuthority(msg.Creator); err != nil {
		return nil, err
	}

	release, err := m.Release.Get(ctx, msg.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(sdkerrors.ErrKeyNotFound, "release %d not found", msg.Id)
		}
		return nil, err
	}

	if msg.EmergencyOk {
		until := m.nowUnix(ctx)
		if msg.EmergencyTtl > 0 {
			until += msg.EmergencyTtl
		}
		release.EmergencyOk = true
		release.EmergencyUntil = until
	} else {
		release.EmergencyOk = false
		release.EmergencyUntil = 0
	}

	if err := m.Release.Set(ctx, release.Id, release); err != nil {
		return nil, err
	}

	return &types.MsgSetEmergencyResponse{}, nil
}

func (m msgServer) ValidateRelease(ctx context.Context, msg *types.MsgValidateRelease) (*types.MsgValidateReleaseResponse, error) {
	if err := m.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}

	if err := m.updateReleaseStatus(ctx, msg.Id, types.Release_VALIDATED); err != nil {
		return nil, err
	}

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

func (m msgServer) updateReleaseStatus(ctx context.Context, id uint64, status types.Release_ReleaseStatus) error {
	release, err := m.Release.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return errorsmod.Wrapf(sdkerrors.ErrKeyNotFound, "release %d not found", id)
		}
		return err
	}

	if release.Status == status {
		return nil
	}

	release.Status = status
	if status == types.Release_REJECTED {
		release.EmergencyOk = false
		release.EmergencyUntil = 0
	}

	if err := m.Release.Set(ctx, release.Id, release); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	eventType := "release_validate"
	if status == types.Release_REJECTED {
		eventType = "release_reject"
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		eventType,
		sdk.NewAttribute("id", fmt.Sprintf("%d", release.Id)),
		sdk.NewAttribute("status", release.Status.String()),
	))

	return nil
}
