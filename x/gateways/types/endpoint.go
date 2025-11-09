package types

import (
	"fmt"
	"regexp"
	"strings"

	errorsmod "cosmossdk.io/errors"
)

func NormalizeGatewayEndpoint(raw string) (string, error) {
	ep := strings.TrimSpace(raw)
	if ep == "" {
		return "", nil
	}
	ep = strings.ToLower(ep)
	if err := ValidateEndpoint(ep); err != nil {
		return "", errorsmod.Wrap(ErrInvalidRequest, err.Error())
	}
	return ep, nil
}

var endpointRe = regexp.MustCompile(`^(?i)([a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9]))\.([a-z]{2,24})$`)

func ValidateEndpoint(s string) error {
	if len(s) == 0 || len(s) > 253 {
		return fmt.Errorf("invalid endpoint length")
	}
	m := endpointRe.FindStringSubmatch(s)
	if m == nil {
		return fmt.Errorf("invalid endpoint format")
	}
	domain := m[1]
	if strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") {
		return fmt.Errorf("hyphen at edge")
	}
	if len(domain) >= 4 && domain[2:4] == "--" {
		return fmt.Errorf("double-hyphen reserved")
	}
	return nil
}
