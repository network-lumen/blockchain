package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

const (
	DefaultTxTaxRate           = "0.01"
	DefaultTxTaxRateBps uint32 = 100

	DefaultInitialRewardPerBlockLumn uint64 = 1
	DefaultSupplyCapLumn             uint64 = 63_072_000
	DefaultDecimals                  uint32 = 6
	DefaultDenom                            = "ulmn"
	DefaultMinSendUlmn               uint64 = 1000

	DefaultBlockTimeSeconds int64 = 4
	secondsPerYear          int64 = 365 * 24 * 60 * 60
	blocksPerYear           int64 = secondsPerYear / DefaultBlockTimeSeconds

	// 4-second blocks with a 4-year halving cadence → (4 years * seconds per year)/4s.
	DefaultHalvingIntervalBlocks uint64 = uint64(blocksPerYear * 4)

	DefaultDistributionIntervalBlocks uint64 = 10
)

func NewParams(
	txTaxRate string,
	initialRewardLumn uint64,
	halvingInterval uint64,
	supplyCapLumn uint64,
	decimals uint32,
	minSendUlmn uint64,
	denom string,
	distributionInterval uint64,
) Params {
	return Params{
		TxTaxRate:                  txTaxRate,
		InitialRewardPerBlockLumn:  initialRewardLumn,
		HalvingIntervalBlocks:      halvingInterval,
		SupplyCapLumn:              supplyCapLumn,
		Decimals:                   decimals,
		MinSendUlmn:                minSendUlmn,
		Denom:                      denom,
		DistributionIntervalBlocks: distributionInterval,
	}
}

func DefaultParams() Params {
	return NewParams(
		DefaultTxTaxRate,
		DefaultInitialRewardPerBlockLumn,
		DefaultHalvingIntervalBlocks,
		DefaultSupplyCapLumn,
		DefaultDecimals,
		DefaultMinSendUlmn,
		DefaultDenom,
		DefaultDistributionIntervalBlocks,
	)
}

func ValidateParams(p Params) error {
	if p.TxTaxRate == "" {
		return fmt.Errorf("tx_tax_rate must be set")
	}

	dec, err := sdkmath.LegacyNewDecFromStr(p.TxTaxRate)
	if err != nil {
		return fmt.Errorf("tx_tax_rate must be a decimal: %w", err)
	}
	if dec.IsNegative() {
		return fmt.Errorf("tx_tax_rate must be >= 0")
	}
	if dec.GT(sdkmath.LegacyNewDec(1)) {
		return fmt.Errorf("tx_tax_rate must be <= 1")
	}
	if p.InitialRewardPerBlockLumn == 0 {
		return fmt.Errorf("initial_reward_per_block_lumn must be > 0")
	}
	if p.HalvingIntervalBlocks == 0 {
		return fmt.Errorf("halving_interval_blocks must be > 0")
	}
	if p.SupplyCapLumn == 0 {
		return fmt.Errorf("supply_cap_lumn must be > 0")
	}
	if p.Decimals == 0 {
		return fmt.Errorf("decimals must be > 0")
	}
	if p.MinSendUlmn == 0 {
		return fmt.Errorf("min_send_ulmn must be > 0")
	}
	if p.Denom == "" {
		return fmt.Errorf("denom must be set")
	}
	if p.InitialRewardPerBlockLumn > p.SupplyCapLumn {
		return fmt.Errorf("initial reward per block exceeds supply cap")
	}
	if p.DistributionIntervalBlocks == 0 {
		return fmt.Errorf("distribution_interval_blocks must be > 0")
	}
	return nil
}

func GetTxTaxRateBps(p Params) uint32 {
	if p.TxTaxRate != "" {
		if dec, err := sdkmath.LegacyNewDecFromStr(p.TxTaxRate); err == nil {
			if dec.IsNegative() {
				return 0
			}
			scaled := dec.MulInt64(10_000).TruncateInt64()
			if scaled < 0 {
				return 0
			}
			if scaled > 10_000 {
				scaled = 10_000
			}
			return uint32(scaled)
		}
	}
	return DefaultTxTaxRateBps
}

func GetTxTaxRateDec(p Params) sdkmath.LegacyDec {
	bps := GetTxTaxRateBps(p)
	if bps == 0 {
		return sdkmath.LegacyNewDec(0)
	}
	return sdkmath.LegacyNewDec(int64(bps)).QuoInt64(10_000)
}
