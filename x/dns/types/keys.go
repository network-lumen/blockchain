package types

import "cosmossdk.io/collections"

var (
	ModuleName = "dns"

	StoreKey = ModuleName

	GovModuleName = "gov"

	ParamsKey       = collections.NewPrefix("params/")
	DomainKey       = collections.NewPrefix("domain/value/")
	AuctionKey      = collections.NewPrefix("auction/value/")
	OpsThisBlockKey = collections.NewPrefix("ops/this_block/")
)
