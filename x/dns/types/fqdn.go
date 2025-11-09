package types

import (
	"strings"
	"unicode"
	"unicode/utf8"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// normalizeLabel lower-cases ASCII input after stripping non-ASCII runes.
func normalizeLabel(s string) string {
	return strings.ToLower(toASCII(s))
}

func toASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r <= unicode.MaxASCII {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ValidateDomainParts(domain, ext string) error {
	if err := validateLabel("domain", domain); err != nil {
		return err
	}
	if err := validateExt(ext); err != nil {
		return err
	}
	if l := len(domain) + 1 + len(ext); l > DNSFQDNMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("fqdn too long: %d > %d", l, DNSFQDNMaxLen)
	}
	return nil
}

func validateLabel(field, val string) error {
	if val == "" {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s required", field)
	}
	if len(val) > DNSLabelMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s too long: %d > %d", field, len(val), DNSLabelMaxLen)
	}
	for _, r := range val {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return sdkerrors.ErrInvalidRequest.Wrapf("%s contains invalid character %q", field, r)
	}
	return nil
}

func validateExt(val string) error {
	if val == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("extension required")
	}
	if len(val) < 2 || len(val) > DNSLabelMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("extension length invalid: %d", len(val))
	}
	for _, r := range val {
		if r < 'a' || r > 'z' {
			return sdkerrors.ErrInvalidRequest.Wrapf("extension contains invalid character %q", r)
		}
	}
	return nil
}

func ValidateRecords(records []*Record) error {
	if len(records) == 0 {
		return nil
	}
	if len(records) > DNSRecordsMax {
		return sdkerrors.ErrInvalidRequest.Wrapf("too many records: %d > %d", len(records), DNSRecordsMax)
	}
	if payload := RecordsPayloadBytes(records); payload > DNSRecordsPayloadMaxBytes {
		return sdkerrors.ErrInvalidRequest.Wrapf("records payload exceeds %d bytes", DNSRecordsPayloadMaxBytes)
	}
	for i, r := range records {
		if r == nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("records[%d] is nil", i)
		}
		if !utf8.ValidString(r.Key) || strings.TrimSpace(r.Key) == "" {
			return sdkerrors.ErrInvalidRequest.Wrapf("records[%d].key must be UTF-8 and non-empty", i)
		}
		if !utf8.ValidString(r.Value) {
			return sdkerrors.ErrInvalidRequest.Wrapf("records[%d].value must be UTF-8", i)
		}
	}
	return nil
}

func RecordsPayloadBytes(records []*Record) int {
	n := 0
	for _, r := range records {
		if r == nil {
			continue
		}
		n += len(r.Key) + len(r.Value)
	}
	return n
}

func NormalizeDomain(domain string) string { return normalizeLabel(domain) }

func NormalizeExt(ext string) string { return normalizeLabel(ext) }

func NormalizeDomainParts(domain, ext string) (string, string) {
	return NormalizeDomain(domain), NormalizeExt(ext)
}
