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
	DefaultPolicy             = PqcPolicy_PQC_POLICY_REQUIRED
	DefaultMinScheme          = SchemeDilithium3
	DefaultAllowAccountRotate = false

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
		Policy:             DefaultPolicy,
		MinScheme:          DefaultMinScheme,
		AllowAccountRotate: DefaultAllowAccountRotate,
		MinBalanceForLink:  DefaultMinBalanceForLink,
		PowDifficultyBits:  DefaultPowDifficultyBits,
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
