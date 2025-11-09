package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

func monthsFromDays(days uint64) uint64 {
	if days == 0 {
		return 1
	}
	months := (days + 29) / 30
	if months == 0 {
		return 1
	}
	return months
}

func multiplierForLength(tiers []*LengthTier, length int) uint32 {
	if len(tiers) == 0 {
		return tierBpsDenom
	}
	for _, tier := range tiers {
		if tier == nil {
			continue
		}
		if tier.MaxLen == 0 || length <= int(tier.MaxLen) {
			return tier.MultiplierBps
		}
	}
	last := tiers[len(tiers)-1]
	if last == nil {
		return tierBpsDenom
	}
	return last.MultiplierBps
}

func applyBps(amount sdkmath.Int, bps uint32) sdkmath.Int {
	if amount.IsZero() {
		return amount
	}
	num := amount.Mul(sdkmath.NewIntFromUint64(uint64(bps)))
	quo := num.Quo(tierBpsDenomInt)
	if num.Mod(tierBpsDenomInt).IsZero() {
		return quo
	}
	return quo.AddRaw(1)
}

// PriceQuote returns the decimal quote (before ceiling) and the integer amount
// to charge for a domain of the supplied dimensions and duration.
func (p Params) PriceQuote(domainLen, extLen int, durationDays uint64) (sdkmath.LegacyDec, sdkmath.Int, error) {
	if p.MinPriceUlmnPerMonth == 0 {
		return sdkmath.LegacyDec{}, sdkmath.Int{}, fmt.Errorf("min_price_ulmn_per_month must be > 0")
	}
	months := monthsFromDays(durationDays)
	base := sdkmath.NewIntFromUint64(p.MinPriceUlmnPerMonth).Mul(sdkmath.NewIntFromUint64(months))

	domainBps := multiplierForLength(p.DomainTiers, domainLen)
	extBps := multiplierForLength(p.ExtTiers, extLen)

	base = applyBps(base, domainBps)
	base = applyBps(base, extBps)

	minDec := sdkmath.LegacyNewDecFromInt(base)
	multiplier, err := sdkmath.LegacyNewDecFromStr(p.BaseFeeDns)
	if err != nil {
		return sdkmath.LegacyDec{}, sdkmath.Int{}, fmt.Errorf("invalid base_fee_dns: %w", err)
	}
	priceDec := minDec.Mul(multiplier)
	priceInt := priceDec.Ceil().TruncateInt()
	return priceDec, priceInt, nil
}
