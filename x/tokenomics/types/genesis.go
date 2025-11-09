package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

func DefaultGenesis() *GenesisState {
	def := DefaultParams()
	return &GenesisState{
		Params:          &def,
		TotalMintedUlmn: "0",
	}
}

func ValidateGenesis(gs *GenesisState) error {
	if gs == nil {
		return fmt.Errorf("genesis state cannot be nil")
	}
	if gs.Params != nil {
		if err := ValidateParams(*gs.Params); err != nil {
			return err
		}
	}
	if gs.TotalMintedUlmn != "" {
		amt, ok := sdkmath.NewIntFromString(gs.TotalMintedUlmn)
		if !ok {
			return fmt.Errorf("invalid total_minted_ulmn: %s", gs.TotalMintedUlmn)
		}
		if amt.IsNegative() {
			return fmt.Errorf("total_minted_ulmn cannot be negative")
		}
	}
	return nil
}
