package types

import (
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	tokenomicstypes "lumen/x/tokenomics/types"
)

const (
	// PQC is mandatory at runtime. The stored policy field is kept for backwards
	// compatibility but the keeper always enforces REQUIRED regardless of the
	// genesis value.
	DefaultPolicy    = PqcPolicy_PQC_POLICY_REQUIRED
	DefaultMinScheme = SchemeDilithium3

	DefaultPowDifficultyBits uint32 = 21
)

func SupportedSchemes() []string {
	return []string{SchemeDilithium3}
}

var DefaultMinBalanceForLink = sdk.NewCoin(
	tokenomicstypes.DefaultDenom,
	sdkmath.NewIntFromUint64(tokenomicstypes.DefaultMinSendUlmn),
)

func DefaultParams() Params {
	return Params{
		Policy:              DefaultPolicy,
		MinScheme:           DefaultMinScheme,
		MinBalanceForLink:   DefaultMinBalanceForLink,
		PowDifficultyBits:   DefaultPowDifficultyBits,
		IbcRelayerAllowlist: []string{},
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

	if err := validateMinBalance(p.MinBalanceForLink); err != nil {
		return err
	}
	if p.PowDifficultyBits > 256 {
		return fmt.Errorf("pow_difficulty_bits must be <= 256")
	}
	if err := validateIBCRelayerAllowlist(p.IbcRelayerAllowlist); err != nil {
		return err
	}

	return nil
}

func validateMinBalance(coin sdk.Coin) error {
	if !coin.IsValid() {
		return fmt.Errorf("min_balance_for_link must be a valid coin")
	}
	if !coin.IsPositive() {
		return fmt.Errorf("min_balance_for_link must be > 0")
	}
	return nil
}

func validateIBCRelayerAllowlist(addrs []string) error {
	seen := make(map[string]struct{}, len(addrs))
	for _, raw := range addrs {
		addr := strings.TrimSpace(raw)
		if addr == "" {
			return fmt.Errorf("ibc_relayer_allowlist contains an empty address")
		}
		if _, err := sdk.AccAddressFromBech32(addr); err != nil {
			return fmt.Errorf("invalid ibc_relayer_allowlist address %q: %w", addr, err)
		}
		if _, ok := seen[addr]; ok {
			return fmt.Errorf("duplicate ibc_relayer_allowlist address %q", addr)
		}
		seen[addr] = struct{}{}
	}
	return nil
}
