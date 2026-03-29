package keeper

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"lumen/x/release/types"
)

var (
	reSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reSemver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func isAllowedURL(u string) bool {
	return types.ValidateArtifactURL(u) == nil
}

func validateReleaseForPublish(p types.Params, r *types.Release) error {
	if strings.TrimSpace(r.Version) == "" {
		return fmt.Errorf("version required")
	}
	if !reSemver.MatchString(r.Version) {
		return fmt.Errorf("invalid semver version")
	}
	if !containsString(p.Channels, r.Channel) {
		return fmt.Errorf("channel not allowed")
	}
	if uint32(len(r.Artifacts)) == 0 || uint32(len(r.Artifacts)) > p.MaxArtifacts {
		return fmt.Errorf("artifacts length out of bounds")
	}
	if uint32(len(r.Notes)) > p.MaxNotesLen {
		return fmt.Errorf("notes too long")
	}
	for i := range r.Artifacts {
		a := r.Artifacts[i]
		if a == nil || strings.TrimSpace(a.Platform) == "" || strings.TrimSpace(a.Kind) == "" {
			return fmt.Errorf("artifact[%d]: platform/kind required", i)
		}
		if !reSHA256.MatchString(strings.ToLower(a.Sha256Hex)) {
			return fmt.Errorf("artifact[%d]: invalid sha256_hex", i)
		}
		if uint32(len(a.Urls)) > p.MaxUrlsPerArt {
			return fmt.Errorf("artifact[%d]: too many urls", i)
		}
		for _, u := range a.Urls {
			if !isAllowedURL(u) {
				return fmt.Errorf("artifact[%d]: invalid url", i)
			}
		}
		if uint32(len(a.Signatures)) > p.MaxSigsPerArt {
			return fmt.Errorf("artifact[%d]: too many signatures", i)
		}
	}
	return nil
}

func (m msgServer) PublishRelease(ctx context.Context, msg *types.MsgPublishRelease) (*types.MsgPublishReleaseResponse, error) {
	params := m.GetParams(ctx)
	if !containsString(params.AllowedPublishers, msg.Creator) {
		return nil, errorsmod.Wrap(types.ErrUnauthorizedPublisher, "creator not allowed")
	}
	creatorBz, err := m.addressCodec.StringToBytes(msg.Creator)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, err.Error())
	}
	creatorAddr := sdk.AccAddress(creatorBz)

	r := msg.Release
	if err := validateReleaseForPublish(params, &r); err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidRequest, "%s", err.Error())
	}

	last, _ := m.ReleaseSeq.Peek(ctx)
	nextID := last + 1

	r.Id = nextID
	r.Publisher = msg.Creator
	r.CreatedAt = m.nowUnix(ctx)
	r.Yanked = false
	r.Status = types.Release_PENDING
	r.EmergencyOk = false
	r.EmergencyUntil = 0

	if err := m.chargeEscrow(ctx, nextID, creatorAddr, params.PublishFeeUlmn); err != nil {
		return nil, err
	}
	if params.MaxPendingTtl > 0 {
		if err := m.enqueueExpiry(ctx, nextID, r.CreatedAt+int64(params.MaxPendingTtl)); err != nil {
			return nil, err
		}
	}

	if err := m.ReleaseSeq.Set(ctx, nextID); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "seq set failed")
	}

	if err := m.Release.Set(ctx, nextID, r); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "store release failed")
	}
	if err := m.ByVersion.Set(ctx, r.Version, nextID); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "index byVersion failed")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"release_publish",
		sdk.NewAttribute("id", fmt.Sprintf("%d", nextID)),
		sdk.NewAttribute("version", r.Version),
		sdk.NewAttribute("channel", r.Channel),
		sdk.NewAttribute("publisher", r.Publisher),
	))

	return &types.MsgPublishReleaseResponse{Id: nextID}, nil
}
