package types

import "cosmossdk.io/collections"

const (
	ModuleName = "tokenomics"
	StoreKey   = ModuleName

	GovModuleName = "gov"
)

var (
	ParamsKey                 = collections.NewPrefix("tokenomics/params")
	TotalMintedKey            = collections.NewPrefix("tokenomics/total_minted")
	GovVoteLastHeightKey      = collections.NewPrefix("tokenomics/gov_vote_last_height")
	WithdrawAddrLastHeightKey = collections.NewPrefix("tokenomics/withdraw_addr_last_height")
)
