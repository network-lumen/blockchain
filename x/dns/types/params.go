package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

const (
	tierBpsDenom = 10_000
)

var (
	tierBpsDenomInt = sdkmath.NewInt(tierBpsDenom)
)

var _ paramtypes.ParamSet = (*Params)(nil)

var (
	KeyUpdateFeeUlmn = []byte("UpdateFeeUlmn")
)

var (
	// DefaultBaseFeeDns is a unitless multiplier (historically LMN/day). It still
	// uses the legacy dynamic controller fields (alpha/floor/ceiling) but applies
	// to the min-price-after-tier calculation.
	DefaultBaseFeeDns string = "1.0"
	// DefaultAlpha controls how aggressively the dynamic fee reacts (±12.5% per block).
	DefaultAlpha string = "0.125"
	// DefaultFloor clamps the dynamic fee lower bound.
	DefaultFloor string = "0.1"
	// DefaultCeiling clamps the dynamic fee upper bound.
	DefaultCeiling string = "100"

	// DefaultT is the target number of DNS operations per block.
	DefaultT           uint64 = 50
	DefaultGraceDays   uint64 = 7
	DefaultAuctionDays uint64 = 7

	DefaultTransferFeeUlmn        uint64 = 100_000 // 0.1 LMN
	DefaultBidFeeUlmn             uint64 = 1_000   // 0.001 LMN
	DefaultUpdateRateLimitSeconds uint64 = 60      // one update per minute
	DefaultUpdatePowDifficulty    uint32 = 12      // ~ 1 in 4096 guesses
	DefaultUpdateFeeUlmn          uint64 = 0       // gasless by default

	// DefaultMinPriceUlmnPerMonth is the DAO-controlled floor before multipliers.
	DefaultMinPriceUlmnPerMonth uint64 = 30_000_000 // 30 LMN / month
)

func NewParams(
	baseFeeDns string,
	alpha string,
	floor string,
	ceiling string,
	t uint64,
	graceDays uint64,
	auctionDays uint64,
	transferFeeUlmn uint64,
	bidFeeUlmn uint64,
	updateRateLimitSeconds uint64,
	updatePowDifficulty uint32,
	updateFeeUlmn uint64,
	domainTiers []*LengthTier,
	extTiers []*LengthTier,
	minPrice uint64,
) Params {
	return Params{
		BaseFeeDns:             baseFeeDns,
		Alpha:                  alpha,
		Floor:                  floor,
		Ceiling:                ceiling,
		T:                      t,
		GraceDays:              graceDays,
		AuctionDays:            auctionDays,
		TransferFeeUlmn:        transferFeeUlmn,
		BidFeeUlmn:             bidFeeUlmn,
		UpdateRateLimitSeconds: updateRateLimitSeconds,
		UpdatePowDifficulty:    updatePowDifficulty,
		UpdateFeeUlmn:          updateFeeUlmn,
		DomainTiers:            cloneLengthTiers(domainTiers),
		ExtTiers:               cloneLengthTiers(extTiers),
		MinPriceUlmnPerMonth:   minPrice,
	}
}

func DefaultParams() Params {
	return NewParams(
		DefaultBaseFeeDns,
		DefaultAlpha,
		DefaultFloor,
		DefaultCeiling,
		DefaultT,
		DefaultGraceDays,
		DefaultAuctionDays,
		DefaultTransferFeeUlmn,
		DefaultBidFeeUlmn,
		DefaultUpdateRateLimitSeconds,
		DefaultUpdatePowDifficulty,
		DefaultUpdateFeeUlmn,
		defaultLengthTiers(defaultDomainTierDefs),
		defaultLengthTiers(defaultExtTierDefs),
		DefaultMinPriceUlmnPerMonth,
	)
}

func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair(KeyUpdateFeeUlmn, &p.UpdateFeeUlmn, validateUpdateFeeParam),
	}
}

func (p Params) Validate() error {
	if err := validateBaseFeeDns(p.BaseFeeDns); err != nil {
		return err
	}
	if err := validateAlpha(p.Alpha); err != nil {
		return err
	}
	if err := validateFloor(p.Floor); err != nil {
		return err
	}
	if err := validateCeiling(p.Ceiling); err != nil {
		return err
	}
	if err := validateT(p.T); err != nil {
		return err
	}
	if err := validateGraceDays(p.GraceDays); err != nil {
		return err
	}
	if err := validateAuctionDays(p.AuctionDays); err != nil {
		return err
	}
	if err := validateTransferFee(p.TransferFeeUlmn); err != nil {
		return err
	}
	if err := validateBidFee(p.BidFeeUlmn); err != nil {
		return err
	}
	if err := validateUpdateRateLimit(p.UpdateRateLimitSeconds); err != nil {
		return err
	}
	if err := validateUpdatePowDifficulty(p.UpdatePowDifficulty); err != nil {
		return err
	}
	if err := validateUpdateFee(p.UpdateFeeUlmn); err != nil {
		return err
	}
	if err := validateLengthTiers("domain_tiers", p.DomainTiers); err != nil {
		return err
	}
	if err := validateLengthTiers("ext_tiers", p.ExtTiers); err != nil {
		return err
	}
	if err := validateMinPrice(p.MinPriceUlmnPerMonth); err != nil {
		return err
	}

	base, e1 := sdkmath.LegacyNewDecFromStr(p.BaseFeeDns)
	floor, e2 := sdkmath.LegacyNewDecFromStr(p.Floor)
	ceil, e3 := sdkmath.LegacyNewDecFromStr(p.Ceiling)
	if e1 == nil && e2 == nil && e3 == nil {
		if floor.GT(ceil) {
			return fmt.Errorf("floor > ceiling")
		}
		if base.LT(floor) || base.GT(ceil) {
			return fmt.Errorf("base_fee_dns must be within [floor, ceiling]")
		}
	}

	return nil
}

func validateDecNonNegative(v string, field string) error {
	if v == "" {
		return fmt.Errorf("%s must be set", field)
	}
	d, err := sdkmath.LegacyNewDecFromStr(v)
	if err != nil {
		return fmt.Errorf("%s must be a decimal: %w", field, err)
	}
	if d.IsNegative() {
		return fmt.Errorf("%s must be >= 0", field)
	}
	return nil
}

func validateBaseFeeDns(v string) error { return validateDecNonNegative(v, "base_fee_dns") }
func validateAlpha(v string) error      { return validateDecNonNegative(v, "alpha") }
func validateFloor(v string) error      { return validateDecNonNegative(v, "floor") }
func validateCeiling(v string) error    { return validateDecNonNegative(v, "ceiling") }

func validateT(v uint64) error {
	return nil
}
func validateGraceDays(v uint64) error   { return nil }
func validateAuctionDays(v uint64) error { return nil }

func validateTransferFee(v uint64) error {
	return nil
}

func validateBidFee(v uint64) error {
	return nil
}

func validateUpdateRateLimit(v uint64) error {
	return nil
}

func validateUpdatePowDifficulty(v uint32) error {
	if v > 256 {
		return fmt.Errorf("update_pow_difficulty must be <= 256 bits")
	}
	return nil
}

func validateUpdateFee(v uint64) error {
	return nil
}

func validateUpdateFeeParam(i interface{}) error {
	v, ok := i.(uint64)
	if !ok {
		return fmt.Errorf("invalid parameter type: %T", i)
	}
	return validateUpdateFee(v)
}

func validateMinPrice(v uint64) error {
	if v == 0 {
		return fmt.Errorf("min_price_ulmn_per_month must be > 0")
	}
	return nil
}

type tierDef struct {
	maxLen uint32
	bps    uint32
}

var defaultDomainTierDefs = []tierDef{
	{maxLen: 4, bps: 40_000},  // <=4 chars → 4.0x
	{maxLen: 8, bps: 20_000},  // 5-8 chars → 2.0x
	{maxLen: 15, bps: 10_000}, // 9-15 chars → 1.0x
	{maxLen: 0, bps: 5_000},   // >15 chars  → 0.5x
}

var defaultExtTierDefs = []tierDef{
	{maxLen: 3, bps: 15_000}, // <=3 chars → 1.5x
	{maxLen: 6, bps: 10_000}, // 4-6 chars → 1.0x
	{maxLen: 0, bps: 7_000},  // >6 chars  → 0.7x
}

func defaultLengthTiers(defs []tierDef) []*LengthTier {
	out := make([]*LengthTier, len(defs))
	for i, def := range defs {
		out[i] = &LengthTier{
			MaxLen:        def.maxLen,
			MultiplierBps: def.bps,
		}
	}
	return out
}

func cloneLengthTiers(src []*LengthTier) []*LengthTier {
	if len(src) == 0 {
		return nil
	}
	out := make([]*LengthTier, len(src))
	for i, tier := range src {
		if tier == nil {
			continue
		}
		copyTier := *tier
		out[i] = &copyTier
	}
	return out
}

func validateLengthTiers(name string, tiers []*LengthTier) error {
	if len(tiers) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	for i, tier := range tiers {
		if tier == nil {
			return fmt.Errorf("%s[%d]: tier must not be nil", name, i)
		}
		if tier.MultiplierBps == 0 {
			return fmt.Errorf("%s[%d]: multiplier_bps must be > 0", name, i)
		}
		if tier.MaxLen == 0 {
			if i != len(tiers)-1 {
				return fmt.Errorf("%s[%d]: max_len=0 is only allowed on the last tier", name, i)
			}
			break
		}
		if i > 0 {
			prev := tiers[i-1]
			if prev == nil {
				return fmt.Errorf("%s[%d]: previous tier nil", name, i)
			}
			if prev.MaxLen == 0 || tier.MaxLen <= prev.MaxLen {
				return fmt.Errorf("%s[%d]: max_len must increase and the final tier must use max_len=0", name, i)
			}
		}
	}
	if tiers[len(tiers)-1].MaxLen != 0 {
		return fmt.Errorf("%s: last tier must set max_len=0", name)
	}
	return nil
}
