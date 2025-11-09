package types

import (
	"regexp"
	"strings"
	"unicode/utf8"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = (*MsgPublishRelease)(nil)
	_ sdk.Msg = (*MsgYankRelease)(nil)
	_ sdk.Msg = (*MsgMirrorRelease)(nil)
	_ sdk.Msg = (*MsgSetEmergency)(nil)
	_ sdk.Msg = (*MsgValidateRelease)(nil)
	_ sdk.Msg = (*MsgRejectRelease)(nil)
	_ sdk.Msg = (*MsgUpdateParams)(nil)
)

var (
	reSemver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	reSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func (msg *MsgPublishRelease) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	return validateReleasePayload(&msg.Release)
}

func (msg *MsgYankRelease) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if msg.Id == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	return nil
}

func (msg *MsgMirrorRelease) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if msg.Id == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	for i, u := range msg.NewUrls {
		if err := validateReleaseURL(u); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("new_urls[%d]: %s", i, err.Error())
		}
	}
	return nil
}

func (msg *MsgSetEmergency) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if msg.Id == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	if msg.EmergencyTtl < 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("emergency_ttl must be >= 0")
	}
	return nil
}

func (msg *MsgValidateRelease) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address (%s)", err)
	}
	if msg.Id == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	return nil
}

func (msg *MsgRejectRelease) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address (%s)", err)
	}
	if msg.Id == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	return nil
}

func (msg *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address (%s)", err)
	}
	return msg.Params.Validate()
}

func validateReleasePayload(r *Release) error {
	if strings.TrimSpace(r.Version) == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("version required")
	}
	if len(r.Version) > ReleaseVersionMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("version too long: %d > %d", len(r.Version), ReleaseVersionMaxLen)
	}
	if !reSemver.MatchString(r.Version) {
		return sdkerrors.ErrInvalidRequest.Wrap("version must be semver")
	}
	if strings.TrimSpace(r.Channel) == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("channel required")
	}
	if len(r.Channel) > ReleaseChannelMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("channel too long: %d > %d", len(r.Channel), ReleaseChannelMaxLen)
	}
	if err := validateNotes(r.Notes); err != nil {
		return err
	}
	if len(r.Artifacts) == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("artifacts required")
	}
	for i, art := range r.Artifacts {
		if art == nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d] is nil", i)
		}
		if err := validateArtifact(i, art); err != nil {
			return err
		}
	}
	return nil
}

func validateArtifact(idx int, a *Artifact) error {
	if err := validateASCIIString("artifact.platform", a.Platform, ReleasePlatformMaxLen); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d]: %s", idx, err.Error())
	}
	if err := validateASCIIString("artifact.kind", a.Kind, ReleaseKindMaxLen); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d]: %s", idx, err.Error())
	}
	if !reSHA256.MatchString(strings.ToLower(strings.TrimSpace(a.Sha256Hex))) {
		return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d]: invalid sha256_hex", idx)
	}
	for j, u := range a.Urls {
		if err := validateReleaseURL(u); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d].urls[%d]: %s", idx, j, err.Error())
		}
	}
	for j, sig := range a.Signatures {
		if sig == nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d].signatures[%d] is nil", idx, j)
		}
		if len(sig.Sig) > ReleaseSignatureMaxLen {
			return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d].signatures[%d]: signature too large", idx, j)
		}
		if err := validateASCIIString("signature.key_id", sig.KeyId, ReleasePlatformMaxLen); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d].signatures[%d]: %s", idx, j, err.Error())
		}
		if err := validateASCIIString("signature.algo", sig.Algo, ReleaseKindMaxLen); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("artifact[%d].signatures[%d]: %s", idx, j, err.Error())
		}
	}
	return nil
}

func validateASCIIString(field, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s required", field)
	}
	if len(value) > max {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s too long: %d > %d", field, len(value), max)
	}
	if !utf8.ValidString(value) {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s must be valid UTF-8", field)
	}
	for _, r := range value {
		if r < 0x20 {
			return sdkerrors.ErrInvalidRequest.Wrapf("%s contains control characters", field)
		}
	}
	return nil
}

func validateNotes(notes string) error {
	if notes == "" {
		return nil
	}
	if len(notes) > ReleaseNotesMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("notes too long: %d > %d", len(notes), ReleaseNotesMaxLen)
	}
	if !utf8.ValidString(notes) {
		return sdkerrors.ErrInvalidRequest.Wrap("notes must be valid UTF-8")
	}
	for _, r := range notes {
		if r < 0x20 && r != '\n' && r != '\t' {
			return sdkerrors.ErrInvalidRequest.Wrap("notes contains disallowed control characters")
		}
	}
	return nil
}

func validateReleaseURL(u string) error {
	return ValidateArtifactURL(u)
}
