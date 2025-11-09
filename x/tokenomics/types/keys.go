package types

import "cosmossdk.io/collections"

const (
	ModuleName = "tokenomics"
	StoreKey   = ModuleName

	GovModuleName = "gov"
)

var (
	ParamsKey      = collections.NewPrefix("tokenomics/params")
	TotalMintedKey = collections.NewPrefix("tokenomics/total_minted")
)
