package app

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

// EnsureZeroMinGasPrices validates that --minimum-gas-prices is unset or exactly 0ulmn.
// Returns an error instead of panicking so callers can decide how to handle failures.
func EnsureZeroMinGasPrices(appOpts servertypes.AppOptions) error {
	if appOpts == nil {
		return nil
	}
	raw := appOpts.Get(server.FlagMinGasPrices)
	if raw == nil {
		return nil
	}

	minGas := strings.TrimSpace(fmt.Sprintf("%v", raw))
	if minGas == "" {
		return nil
	}

	if strings.EqualFold(minGas, "0ulmn") {
		return nil
	}

	return fmt.Errorf("gasless chain: minimum-gas-prices must be 0ulmn or unset (got %q)", minGas)
}
