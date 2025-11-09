package types

import (
	"fmt"
	"strings"
)

const (
	// PQC is mandatory at runtime. The stored policy field is kept for backwards
	// compatibility but the keeper always enforces REQUIRED regardless of the
	// genesis value.
	DefaultPolicy             = PqcPolicy_PQC_POLICY_REQUIRED
	DefaultMinScheme          = SchemeDilithium3
	DefaultAllowAccountRotate = false
)

func SupportedSchemes() []string {
	return []string{SchemeDilithium3}
}

func DefaultParams() Params {
	return Params{
		Policy:             DefaultPolicy,
		MinScheme:          DefaultMinScheme,
		AllowAccountRotate: DefaultAllowAccountRotate,
	}
}

func (p Params) Validate() error {
	if p.Policy != PqcPolicy_PQC_POLICY_REQUIRED {
		return fmt.Errorf("pqc policy must be REQUIRED")
	}

	supported := make(map[string]struct{}, len(SupportedSchemes()))
	for _, name := range SupportedSchemes() {
		supported[strings.ToLower(name)] = struct{}{}
	}
	if _, ok := supported[strings.ToLower(p.MinScheme)]; !ok {
		return fmt.Errorf("unsupported min_scheme %q", p.MinScheme)
	}

	return nil
}
