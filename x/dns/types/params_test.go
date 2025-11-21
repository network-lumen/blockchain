package types

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

func TestDefaultParamsValidate(t *testing.T) {
	require.NoError(t, DefaultParams().Validate())
}

func TestParamsValidateLengthTiers(t *testing.T) {
	p := DefaultParams()
	p.DomainTiers = nil
	require.Error(t, p.Validate())

	p = DefaultParams()
	p.DomainTiers = []*LengthTier{
		{MaxLen: 4, MultiplierBps: 0},
		{MaxLen: 0, MultiplierBps: tierBpsDenom},
	}
	require.Error(t, p.Validate())

	p = DefaultParams()
	p.ExtTiers = []*LengthTier{
		{MaxLen: 5, MultiplierBps: 10_000},
		{MaxLen: 5, MultiplierBps: 8_000},
		{MaxLen: 0, MultiplierBps: 7_000},
	}
	require.Error(t, p.Validate())
}

func TestPriceQuote(t *testing.T) {
	p := DefaultParams()

	_, amt, err := p.PriceQuote(8, 5, 365)
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(52000000), amt)

	_, amt, err = p.PriceQuote(3, 2, 180)
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(72000000), amt)
}
