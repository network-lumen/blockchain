package app

import (
	"slices"

	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/rootmulti"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"

	ibctransfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibctransferkeeper "github.com/cosmos/ibc-go/v10/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	transferv2 "github.com/cosmos/ibc-go/v10/modules/apps/transfer/v2"
	ibc "github.com/cosmos/ibc-go/v10/modules/core"
	porttypes "github.com/cosmos/ibc-go/v10/modules/core/05-port/types"
	ibcapi "github.com/cosmos/ibc-go/v10/modules/core/api"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"
)

const ibcUpgradeName = "v1.5.0"

var ibcCompatibleUpgradeNames = map[string]struct{}{
	ibcUpgradeName: {},
	"v1.5.2":       {},
}

type legacyIBCSubspace struct{}

func (legacyIBCSubspace) GetParamSet(_ sdk.Context, _ paramtypes.ParamSet) {}

func (app *App) registerIBC() {
	ibc.AppModuleBasic{}.RegisterInterfaces(app.interfaceRegistry)
	ibctransfer.AppModuleBasic{}.RegisterInterfaces(app.interfaceRegistry)
	ibctm.AppModuleBasic{}.RegisterInterfaces(app.interfaceRegistry)

	ibcStoreKey := storetypes.NewKVStoreKey(ibcexported.StoreKey)
	transferStoreKey := storetypes.NewKVStoreKey(ibctransfertypes.StoreKey)

	if err := app.RegisterStores(ibcStoreKey, transferStoreKey); err != nil {
		panic(err)
	}

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	app.IBCKeeper = ibckeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(ibcStoreKey),
		legacyIBCSubspace{},
		app.UpgradeKeeper,
		authority,
	)

	app.TransferKeeper = ibctransferkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(transferStoreKey),
		legacyIBCSubspace{},
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.ChannelKeeper,
		app.MsgServiceRouter(),
		app.AuthKeeper,
		app.BankKeeper,
		authority,
	)

	ibcRouter := porttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, ibctransfer.NewIBCModule(app.TransferKeeper))
	app.IBCKeeper.SetRouter(ibcRouter)

	ibcRouterV2 := ibcapi.NewRouter()
	ibcRouterV2.AddRoute(ibctransfertypes.PortID, transferv2.NewIBCModule(app.TransferKeeper))
	app.IBCKeeper.SetRouterV2(ibcRouterV2)

	storeProvider := app.IBCKeeper.ClientKeeper.GetStoreProvider()
	tmLightClientModule := ibctm.NewLightClientModule(app.appCodec, storeProvider)
	app.IBCKeeper.ClientKeeper.AddRoute(ibctm.ModuleName, &tmLightClientModule)
}

func (app *App) registerIBCModules() {
	if err := app.RegisterModules(
		ibc.NewAppModule(app.IBCKeeper),
		ibctransfer.NewAppModule(app.TransferKeeper),
	); err != nil {
		panic(err)
	}

	app.configureIBCModuleOrder()
}

func (app *App) configureStoreLoaders(db dbm.DB) {
	if app.UpgradeKeeper == nil {
		return
	}

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}
	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}
	if !isIBCCompatibleUpgradePlan(upgradeInfo.Name) {
		return
	}

	needsStoreLoader, err := ibcStoresMissingAtLatestVersion(db)
	if err != nil {
		panic(err)
	}
	if !needsStoreLoader {
		return
	}

	app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storetypes.StoreUpgrades{
		Added: []string{
			ibcexported.StoreKey,
			ibctransfertypes.StoreKey,
		},
	}))
}

func isIBCCompatibleUpgradePlan(name string) bool {
	_, ok := ibcCompatibleUpgradeNames[name]
	return ok
}

func ibcStoresMissingAtLatestVersion(db dbm.DB) (bool, error) {
	latestVersion := rootmulti.GetLatestVersion(db)
	if latestVersion == 0 {
		return true, nil
	}

	store := rootmulti.NewStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	commitInfo, err := store.GetCommitInfo(latestVersion)
	if err != nil {
		return false, err
	}

	hasIBCStore := false
	hasTransferStore := false
	for _, storeInfo := range commitInfo.StoreInfos {
		switch storeInfo.Name {
		case ibcexported.StoreKey:
			hasIBCStore = true
		case ibctransfertypes.StoreKey:
			hasTransferStore = true
		}
	}

	return !hasIBCStore || !hasTransferStore, nil
}

func (app *App) configureIBCModuleOrder() {
	app.ModuleManager.SetOrderInitGenesis(appendUnique(app.ModuleManager.OrderInitGenesis, ibcexported.ModuleName, ibctransfertypes.ModuleName)...)
	app.ModuleManager.SetOrderExportGenesis(appendUnique(app.ModuleManager.OrderExportGenesis, ibcexported.ModuleName, ibctransfertypes.ModuleName)...)
	app.ModuleManager.SetOrderBeginBlockers(insertAfter(app.ModuleManager.OrderBeginBlockers, govtypes.ModuleName, ibcexported.ModuleName)...)
}

func appendUnique(values []string, additions ...string) []string {
	out := slices.Clone(values)
	for _, addition := range additions {
		if !slices.Contains(out, addition) {
			out = append(out, addition)
		}
	}
	return out
}

func insertAfter(values []string, anchor string, additions ...string) []string {
	out := appendUnique(values, additions...)
	index := slices.Index(out, anchor)
	if index == -1 {
		return out
	}

	filtered := make([]string, 0, len(out))
	for _, value := range out {
		if slices.Contains(additions, value) {
			continue
		}
		filtered = append(filtered, value)
	}

	index = slices.Index(filtered, anchor)
	if index == -1 {
		return appendUnique(filtered, additions...)
	}

	result := make([]string, 0, len(filtered)+len(additions))
	result = append(result, filtered[:index+1]...)
	result = append(result, additions...)
	result = append(result, filtered[index+1:]...)
	return result
}
