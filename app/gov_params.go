package app

import (
	sdkmath "cosmossdk.io/math"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

// harden x/gov quorum/threshold defaults before any module initialises.
func init() {
	govv1.DefaultQuorum = sdkmath.LegacyMustNewDecFromStr("0.67")
	govv1.DefaultThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")
	govv1.DefaultExpeditedThreshold = sdkmath.LegacyMustNewDecFromStr("0.85")
	govv1.DefaultVetoThreshold = sdkmath.LegacyMustNewDecFromStr("0.334")
}
