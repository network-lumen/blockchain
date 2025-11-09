package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	authmodule "github.com/cosmos/cosmos-sdk/x/auth"
	bankmodule "github.com/cosmos/cosmos-sdk/x/bank"
	distrmodule "github.com/cosmos/cosmos-sdk/x/distribution"
	genutilmodule "github.com/cosmos/cosmos-sdk/x/genutil"
	stakingmodule "github.com/cosmos/cosmos-sdk/x/staking"

	dnsmodule "lumen/x/dns/module"
	gatewaysmodule "lumen/x/gateways/module"
	pqcmodule "lumen/x/pqc/module"
	releasemodule "lumen/x/release/module"
	tokenomicsmodule "lumen/x/tokenomics/module"
)

// ModuleBasics exposes AppModuleBasic registrations for preflight and tooling.
var ModuleBasics = module.NewBasicManager(
	authmodule.AppModuleBasic{},
	bankmodule.AppModuleBasic{},
	distrmodule.AppModuleBasic{},
	stakingmodule.AppModuleBasic{},
	genutilmodule.AppModuleBasic{},
	dnsmodule.AppModule{},
	gatewaysmodule.AppModule{},
	pqcmodule.AppModule{},
	releasemodule.AppModule{},
	tokenomicsmodule.AppModule{},
)
