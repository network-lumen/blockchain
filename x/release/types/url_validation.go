package types

import (
	"net/url"
	"strings"
	"unicode"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	dnstypes "lumen/x/dns/types"
)

const maxAllowedURLLen = 1024

// ValidateArtifactURL enforces the allowed URL schemes for release artifacts.
func ValidateArtifactURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("url required")
	}
	if len(raw) > maxAllowedURLLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("url too long: %d > %d", len(raw), maxAllowedURLLen)
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "ipfs://"):
		return validateIPFSCID(strings.TrimSpace(raw[len("ipfs://"):]))
	case strings.HasPrefix(lower, "lumen://ipfs/"):
		return validateIPFSCID(strings.TrimSpace(raw[len("lumen://ipfs/"):]))
	case strings.HasPrefix(lower, "lumen://ipns/"):
		return validateIPNSName(strings.TrimSpace(raw[len("lumen://ipns/"):]))
	case strings.HasPrefix(lower, "lumen://"):
		return validateLumenFQDN(strings.TrimSpace(raw[len("lumen://"):]))
	default:
		return validateHTTPURL(raw)
	}
}

func validateHTTPURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap("url parse error")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return sdkerrors.ErrInvalidRequest.Wrap("url scheme must be http/https/ipfs/lumen")
	}
	if parsed.Host == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("url host required")
	}
	return nil
}

func validateIPFSCID(cid string) error {
	if cid == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("ipfs cid required")
	}
	if len(cid) > 128 {
		return sdkerrors.ErrInvalidRequest.Wrap("ipfs cid too long")
	}
	for _, r := range cid {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return sdkerrors.ErrInvalidRequest.Wrapf("ipfs cid contains invalid character %q", r)
	}
	return nil
}

func validateIPNSName(name string) error {
	if name == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("ipns name required")
	}
	if len(name) > 255 {
		return sdkerrors.ErrInvalidRequest.Wrap("ipns name too long")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return sdkerrors.ErrInvalidRequest.Wrapf("ipns name contains invalid character %q", r)
	}
	return nil
}

func validateLumenFQDN(host string) error {
	if host == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("fqdn required")
	}
	if strings.ContainsAny(host, "/?#") {
		return sdkerrors.ErrInvalidRequest.Wrap("fqdn cannot contain path or query segments")
	}
	dot := strings.IndexByte(host, '.')
	if dot <= 0 || dot == len(host)-1 {
		return sdkerrors.ErrInvalidRequest.Wrap("fqdn must be domain.ext")
	}
	domain := normalizeASCII(host[:dot])
	ext := normalizeASCII(host[dot+1:])
	return dnstypes.ValidateDomainParts(domain, ext)
}

func normalizeASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r <= unicode.MaxASCII {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
