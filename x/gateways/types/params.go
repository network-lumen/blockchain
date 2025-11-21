package types

import "fmt"

const (
	defaultActionFeeUlmn          uint64 = 1_000      // 0.001 LUMEN
	defaultRegisterGatewayFeeUlmn uint64 = 50_000_000 // 50 LUMEN
	defaultMinContractPriceUlmn   uint64 = 100_000    // 0.1 LUMEN / month
)

func NewParams() Params {
	p := DefaultParams()
	return p
}

func DefaultParams() Params {
	return Params{
		PlatformCommissionBps:        100,
		MonthSeconds:                 30 * 24 * 60 * 60,
		FinalizeDelayMonths:          12,
		FinalizerRewardBps:           500,
		MinPriceUlmnPerMonth:         defaultMinContractPriceUlmn,
		MaxActiveContractsPerGateway: 100,
		ActionFeeUlmn:                defaultActionFeeUlmn,
		RegisterGatewayFeeUlmn:       defaultRegisterGatewayFeeUlmn,
	}
}

func ValidateParams(p Params) error {
	if p.MonthSeconds == 0 {
		return fmt.Errorf("month_seconds must be > 0")
	}
	if p.FinalizerRewardBps > 10_000 {
		return fmt.Errorf("finalizer_reward_bps must be <= 10000")
	}
	if p.PlatformCommissionBps > 10_000 {
		return fmt.Errorf("platform_commission_bps must be <= 10000")
	}
	if p.MaxActiveContractsPerGateway == 0 {
		return fmt.Errorf("max_active_contracts_per_gateway must be > 0")
	}
	if p.MinPriceUlmnPerMonth == 0 {
		return fmt.Errorf("min_price_ulmn_per_month must be > 0")
	}
	if p.ActionFeeUlmn > 1_000_000_000_000_000 {
		return fmt.Errorf("action_fee_ulmn is too large")
	}
	if p.RegisterGatewayFeeUlmn > 1_000_000_000_000_000 {
		return fmt.Errorf("register_gateway_fee_ulmn is too large")
	}
	return nil
}
