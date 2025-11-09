package types

import "cosmossdk.io/collections"

const (
	ModuleName    = "gateways"
	StoreKey      = ModuleName
	GovModuleName = "gov"

	ModuleAccountTreasury = "GatewaysTreasury"
	ModuleAccountEscrow   = "GatewaysEscrow"
)

var ParamsKey = collections.NewPrefix("gateways/params")

var (
	GatewayKey     = collections.NewPrefix("gateways/gateway/")
	GatewaySeqKey  = collections.NewPrefix("gateways/gateway_seq")
	ContractKey    = collections.NewPrefix("gateways/contract/")
	ContractSeqKey = collections.NewPrefix("gateways/contract_seq")
)
