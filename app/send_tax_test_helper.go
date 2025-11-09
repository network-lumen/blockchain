package app

import (
	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// TestOnlyComputeSendTaxes exposes computeSendTaxes to tests without exporting it to production code.
func TestOnlyComputeSendTaxes(tx sdk.Tx, rate sdkmath.LegacyDec, addrCodec address.Codec) (map[string]sdk.Coins, sdk.Coins, error) {
	perPayer, total, err := computeSendTaxes(tx, rate, addrCodec)
	if err != nil || len(perPayer) == 0 {
		return nil, total, err
	}

	out := make(map[string]sdk.Coins, len(perPayer))
	for _, rec := range perPayer {
		out[rec.addr.String()] = rec.coins
	}
	return out, total, nil
}
