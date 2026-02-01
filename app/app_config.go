package app

import (
	_ "lumen/x/dns/module"
	dnsmoduletypes "lumen/x/dns/types"
	_ "lumen/x/gateways/module"
	gatewaysmoduletypes "lumen/x/gateways/types"
	_ "lumen/x/pqc/module"
	pqcmoduletypes "lumen/x/pqc/types"
	_ "lumen/x/release/module"
	releasemoduletypes "lumen/x/release/types"
	_ "lumen/x/tokenomics/module"
	tokenomicsmoduletypes "lumen/x/tokenomics/types"

	runtimev1alpha1 "cosmossdk.io/api/cosmos/app/runtime/v1alpha1"
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	bankmodulev1 "cosmossdk.io/api/cosmos/bank/module/v1"
	consensusmodulev1 "cosmossdk.io/api/cosmos/consensus/module/v1"
	distrmodulev1 "cosmossdk.io/api/cosmos/distribution/module/v1"
	genutilmodulev1 "cosmossdk.io/api/cosmos/genutil/module/v1"
	govmodulev1 "cosmossdk.io/api/cosmos/gov/module/v1"
	slashingmodulev1 "cosmossdk.io/api/cosmos/slashing/module/v1"
	stakingmodulev1 "cosmossdk.io/api/cosmos/staking/module/v1"
	txconfigv1 "cosmossdk.io/api/cosmos/tx/config/v1"
	upgrademodulev1 "cosmossdk.io/api/cosmos/upgrade/module/v1"
	"cosmossdk.io/depinject/appconfig"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	_ "github.com/cosmos/cosmos-sdk/x/bank" // import for side-effects
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	_ "github.com/cosmos/cosmos-sdk/x/consensus" // import for side-effects
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	_ "github.com/cosmos/cosmos-sdk/x/distribution" // import for side-effects
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	_ "github.com/cosmos/cosmos-sdk/x/staking" // import for side-effects
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	_ "github.com/cosmos/cosmos-sdk/x/auth"           // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/auth/tx/config" // import for side-effects
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
)

const immutableAuthorityModuleName = "gov-immutable"

var (
	moduleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: authtypes.FeeCollectorName},
		{Account: distrtypes.ModuleName},
		{Account: dnsmoduletypes.ModuleName},
		{Account: releasemoduletypes.ModuleName},
		{Account: govtypes.ModuleName, Permissions: []string{authtypes.Burner}},
		{Account: gatewaysmoduletypes.ModuleAccountTreasury},
		{Account: gatewaysmoduletypes.ModuleAccountEscrow},
		{Account: tokenomicsmoduletypes.ModuleName, Permissions: []string{authtypes.Minter}},
		{Account: minttypes.ModuleName, Permissions: []string{authtypes.Minter}},
		{Account: stakingtypes.BondedPoolName, Permissions: []string{authtypes.Burner, stakingtypes.ModuleName}},
		{Account: stakingtypes.NotBondedPoolName, Permissions: []string{authtypes.Burner, stakingtypes.ModuleName}},
	}

	blockAccAddrs = []string{
		authtypes.FeeCollectorName,
		distrtypes.ModuleName,
		govtypes.ModuleName,
		stakingtypes.BondedPoolName,
		stakingtypes.NotBondedPoolName,
		tokenomicsmoduletypes.ModuleName,
	}

	immutableAuthority = mustModuleAuthority(immutableAuthorityModuleName)

	appConfig = appconfig.Compose(&appv1alpha1.Config{
		Modules: []*appv1alpha1.ModuleConfig{
			{
				Name: runtime.ModuleName,
				Config: appconfig.WrapAny(&runtimev1alpha1.Module{
					AppName: Name,
					PreBlockers: []string{
						upgradetypes.ModuleName,
						authtypes.ModuleName,
					},
					BeginBlockers: []string{
						distrtypes.ModuleName,
						slashingtypes.ModuleName,
						stakingtypes.ModuleName,
						govtypes.ModuleName,
						dnsmoduletypes.ModuleName,
						releasemoduletypes.ModuleName,
						gatewaysmoduletypes.ModuleName,
						pqcmoduletypes.ModuleName,
						tokenomicsmoduletypes.ModuleName,
					},
					EndBlockers: []string{
						stakingtypes.ModuleName,
						slashingtypes.ModuleName,
						govtypes.ModuleName,
						dnsmoduletypes.ModuleName,
						releasemoduletypes.ModuleName,
						gatewaysmoduletypes.ModuleName,
						pqcmoduletypes.ModuleName,
						tokenomicsmoduletypes.ModuleName,
					},
					OverrideStoreKeys: []*runtimev1alpha1.StoreKeyConfig{
						{
							ModuleName: authtypes.ModuleName,
							KvStoreKey: "acc",
						},
					},
					InitGenesis: []string{
						consensustypes.ModuleName,
						authtypes.ModuleName,
						banktypes.ModuleName,
						distrtypes.ModuleName,
						upgradetypes.ModuleName,
						slashingtypes.ModuleName,
						stakingtypes.ModuleName,
						govtypes.ModuleName,
						genutiltypes.ModuleName,
						dnsmoduletypes.ModuleName,
						releasemoduletypes.ModuleName,
						gatewaysmoduletypes.ModuleName,
						pqcmoduletypes.ModuleName,
						tokenomicsmoduletypes.ModuleName,
					},
				}),
			},
			{
				Name: authtypes.ModuleName,
				Config: appconfig.WrapAny(&authmodulev1.Module{
					Bech32Prefix:                AccountAddressPrefix,
					ModuleAccountPermissions:    moduleAccPerms,
					EnableUnorderedTransactions: false,
					Authority:                   immutableAuthority,
				}),
			},
			{
				Name: banktypes.ModuleName,
				Config: appconfig.WrapAny(&bankmodulev1.Module{
					BlockedModuleAccountsOverride: blockAccAddrs,
					Authority:                     immutableAuthority,
				}),
			},
			{
				Name: slashingtypes.ModuleName,
				Config: appconfig.WrapAny(&slashingmodulev1.Module{
					Authority: immutableAuthority,
				}),
			},
			{
				Name: stakingtypes.ModuleName,
				Config: appconfig.WrapAny(&stakingmodulev1.Module{
					Bech32PrefixValidator: AccountAddressPrefix + "valoper",
					Bech32PrefixConsensus: AccountAddressPrefix + "valcons",
					Authority:             immutableAuthority,
				}),
			},
			{
				Name:   "tx",
				Config: appconfig.WrapAny(&txconfigv1.Config{}),
			},
			{
				Name:   genutiltypes.ModuleName,
				Config: appconfig.WrapAny(&genutilmodulev1.Module{}),
			},
			{
				Name: distrtypes.ModuleName,
				Config: appconfig.WrapAny(&distrmodulev1.Module{
					Authority: immutableAuthority,
				}),
			},
			{
				Name: upgradetypes.ModuleName,
				Config: appconfig.WrapAny(&upgrademodulev1.Module{
					Authority: mustModuleAuthority(govtypes.ModuleName),
				}),
			},
			{
				Name: consensustypes.ModuleName,
				Config: appconfig.WrapAny(&consensusmodulev1.Module{
					Authority: immutableAuthority,
				}),
			},
			{
				Name: govtypes.ModuleName,
				Config: appconfig.WrapAny(&govmodulev1.Module{
					MaxMetadataLen: 4096,
					Authority:      immutableAuthority,
				}),
			},
			{
				Name:   dnsmoduletypes.ModuleName,
				Config: appconfig.WrapAny(&dnsmoduletypes.Module{}),
			},
			{
				Name:   releasemoduletypes.ModuleName,
				Config: appconfig.WrapAny(&releasemoduletypes.Module{}),
			},
			{
				Name:   gatewaysmoduletypes.ModuleName,
				Config: appconfig.WrapAny(&gatewaysmoduletypes.Module{}),
			},
			{
				Name:   pqcmoduletypes.ModuleName,
				Config: appconfig.WrapAny(&pqcmoduletypes.Module{}),
			},
			{
				Name:   tokenomicsmoduletypes.ModuleName,
				Config: appconfig.WrapAny(&tokenomicsmoduletypes.Module{}),
			},
		},
	})
)

func mustModuleAuthority(moduleName string) string {
	addr := authtypes.NewModuleAddress(moduleName)
	return sdk.MustBech32ifyAddressBytes(AccountAddressPrefix, addr)
}
