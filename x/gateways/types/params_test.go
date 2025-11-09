package types

import "testing"

func TestValidateParams(t *testing.T) {
	p := DefaultParams()
	if err := ValidateParams(p); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	bad := p
	bad.MonthSeconds = 0
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for zero month_seconds")
	}

	bad = p
	bad.PlatformCommissionBps = 20000
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for platform commission overflow")
	}

	bad = p
	bad.FinalizerRewardBps = 20000
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for finalizer reward overflow")
	}

	bad = p
	bad.MaxActiveContractsPerGateway = 0
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for zero max active contracts")
	}

	bad = p
	bad.MinPriceUlmnPerMonth = 0
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for zero min price")
	}

	bad = p
	bad.ActionFeeUlmn = 2_000_000_000_000_000
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for oversized action fee")
	}

	bad = p
	bad.RegisterGatewayFeeUlmn = 2_000_000_000_000_000
	if err := ValidateParams(bad); err == nil {
		t.Fatalf("expected error for oversized register fee")
	}

	p.RegisterGatewayFeeUlmn = 0
	if err := ValidateParams(p); err != nil {
		t.Fatalf("zero register fee should be valid: %v", err)
	}
}
