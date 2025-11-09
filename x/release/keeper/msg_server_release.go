package keeper

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cosmossdk.io/collections"
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

func dedupeStrings(in []string, limit uint32) ([]string, uint32) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
		if limit > 0 && uint32(len(out)) >= limit {
			break
		}
	}
	return out, uint32(len(in)) - uint32(len(out))
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
	if _, err := m.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, err.Error())
	}

	r := msg.Release
	if err := validateReleaseForPublish(params, &r); err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidRequest, "%s", err.Error())
	}

	// fee escrow will be added when params + proto updated

	last, _ := m.ReleaseSeq.Peek(ctx)
	nextID := last + 1
	if err := m.ReleaseSeq.Set(ctx, nextID); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "seq set failed")
	}

	r.Id = nextID
	r.Publisher = msg.Creator
	r.CreatedAt = m.nowUnix(ctx)
	r.Yanked = false

	if err := m.Release.Set(ctx, nextID, r); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "store release failed")
	}
	if err := m.ByVersion.Set(ctx, r.Version, nextID); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "index byVersion failed")
	}
	for _, a := range r.Artifacts {
		k := tripleKey(r.Channel, a.Platform, a.Kind)
		if err := m.ByTriple.Set(ctx, k, nextID); err != nil {
			return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "index byTriple failed")
		}
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

func (m msgServer) YankRelease(ctx context.Context, msg *types.MsgYankRelease) (*types.MsgYankReleaseResponse, error) {
	params := m.GetParams(ctx)
	if !containsString(params.AllowedPublishers, msg.Creator) {
		return nil, errorsmod.Wrap(types.ErrUnauthorizedPublisher, "creator not allowed")
	}
	if _, err := m.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, err.Error())
	}

	r, err := m.Release.Get(ctx, msg.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(sdkerrors.ErrKeyNotFound, "release %d not found", msg.Id)
		}
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}
	if r.Yanked {
		return &types.MsgYankReleaseResponse{}, nil
	}
	r.Yanked = true
	if err := m.Release.Set(ctx, r.Id, r); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "update release failed")
	}

	for _, a := range r.Artifacts {
		triple := tripleKey(r.Channel, a.Platform, a.Kind)
		var latest uint64
		last, _ := m.ReleaseSeq.Peek(ctx)
		for id := last; id >= 1; id-- {
			rr, err := m.Release.Get(ctx, id)
			if err != nil {
				continue
			}
			if rr.Channel != r.Channel || rr.Yanked {
				continue
			}
			matched := false
			for _, aa := range rr.Artifacts {
				if strings.EqualFold(aa.Platform, a.Platform) && strings.EqualFold(aa.Kind, a.Kind) {
					matched = true
					break
				}
			}
			if matched {
				latest = id
				break
			}
			if id == 1 {
				break
			}
		}
		if latest == 0 {
			_ = m.ByTriple.Remove(ctx, triple)
		} else {
			_ = m.ByTriple.Set(ctx, triple, latest)
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"release_yank",
		sdk.NewAttribute("id", fmt.Sprintf("%d", r.Id)),
	))

	return &types.MsgYankReleaseResponse{}, nil
}

func (m msgServer) MirrorRelease(ctx context.Context, msg *types.MsgMirrorRelease) (*types.MsgMirrorReleaseResponse, error) {
	params := m.GetParams(ctx)
	if !containsString(params.AllowedPublishers, msg.Creator) {
		return nil, errorsmod.Wrap(types.ErrUnauthorizedPublisher, "creator not allowed")
	}
	if _, err := m.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, err.Error())
	}

	r, err := m.Release.Get(ctx, msg.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(sdkerrors.ErrKeyNotFound, "release %d not found", msg.Id)
		}
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, err.Error())
	}
	if int(msg.ArtifactIndex) < 0 || int(msg.ArtifactIndex) >= len(r.Artifacts) {
		return nil, errorsmod.Wrapf(types.ErrInvalidRequest, "artifact_index out of range")
	}

	urls := make([]string, 0, len(msg.NewUrls))
	for _, u := range msg.NewUrls {
		u = strings.TrimSpace(u)
		if !isAllowedURL(u) {
			return nil, errorsmod.Wrap(types.ErrInvalidRequest, "invalid url")
		}
		urls = append(urls, u)
	}
	urls, _ = dedupeStrings(urls, params.MaxUrlsPerArt)
	art := r.Artifacts[msg.ArtifactIndex]
	mergedMap := map[string]struct{}{}
	for _, u := range art.Urls {
		mergedMap[u] = struct{}{}
	}
	for _, u := range urls {
		mergedMap[u] = struct{}{}
	}
	if uint32(len(mergedMap)) > params.MaxUrlsPerArt {
		return nil, errorsmod.Wrapf(types.ErrInvalidRequest, "too many urls: %d > %d", len(mergedMap), params.MaxUrlsPerArt)
	}
	merged := append([]string{}, art.Urls...)
	existing := map[string]struct{}{}
	for _, u := range art.Urls {
		existing[u] = struct{}{}
	}
	for _, u := range urls {
		if _, ok := existing[u]; !ok {
			merged = append(merged, u)
		}
	}
	addedCount := uint32(0)
	if len(merged) > len(art.Urls) {
		addedCount = uint32(len(merged) - len(art.Urls))
	}
	art.Urls = merged
	r.Artifacts[msg.ArtifactIndex] = art

	if err := m.Release.Set(ctx, r.Id, r); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "update release failed")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"release_mirror",
		sdk.NewAttribute("id", fmt.Sprintf("%d", r.Id)),
		sdk.NewAttribute("artifact_index", fmt.Sprintf("%d", msg.ArtifactIndex)),
		sdk.NewAttribute("added_urls_count", fmt.Sprintf("%d", addedCount)),
	))

	return &types.MsgMirrorReleaseResponse{Added: addedCount}, nil
}
