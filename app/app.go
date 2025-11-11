package app

import (
	"io"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	"lumen/docs"
	dnsmodulekeeper "lumen/x/dns/keeper"
	gatewaysmodulekeeper "lumen/x/gateways/keeper"
	pqcmodulekeeper "lumen/x/pqc/keeper"
	releasemodulekeeper "lumen/x/release/keeper"
	tokenomicsmodulekeeper "lumen/x/tokenomics/keeper"
)

const (
	Name                 = "lumen"
	AccountAddressPrefix = "lmn"
	ChainCoinType        = 118
)

var gaslessMsgTypes = []string{
	"/lumen.gateway.v1.MsgCreateContract",
	"/lumen.gateway.v1.MsgRegisterGateway",
	"/lumen.gateway.v1.MsgUpdateGateway",
	"/lumen.gateway.v1.MsgClaimPayment",
	"/lumen.gateway.v1.MsgCancelContract",
	"/lumen.gateway.v1.MsgFinalizeContract",

	"/lumen.dns.v1.MsgRegister",
	"/lumen.dns.v1.MsgRenew",
	"/lumen.dns.v1.MsgTransfer",
	"/lumen.dns.v1.MsgUpdate",
	"/lumen.dns.v1.MsgBid",
	"/lumen.dns.v1.MsgSettle",
}

// GaslessMsgTypes exposes the currently whitelisted gasless message URLs.
func GaslessMsgTypes() []string {
	out := make([]string, len(gaslessMsgTypes))
	copy(out, gaslessMsgTypes)
	return out
}

var DefaultNodeHome string

var (
	_ runtime.AppI            = (*App)(nil)
	_ servertypes.Application = (*App)(nil)
)

type App struct {
	*runtime.App
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry

	AuthKeeper    authkeeper.AccountKeeper
	BankKeeper    bankkeeper.Keeper
	StakingKeeper *stakingkeeper.Keeper
	DistrKeeper   distrkeeper.Keeper

	DnsKeeper        dnsmodulekeeper.Keeper
	ReleaseKeeper    releasemodulekeeper.Keeper
	GatewaysKeeper   gatewaysmodulekeeper.Keeper
	TokenomicsKeeper tokenomicsmodulekeeper.Keeper
	PqcKeeper        pqcmodulekeeper.Keeper

	sm *module.SimulationManager

	AppOpts servertypes.AppOptions
}

func init() {
	sdk.DefaultBondDenom = "ulmn"

	var err error
	clienthelpers.EnvPrefix = Name
	DefaultNodeHome, err = clienthelpers.GetNodeHomeDirectory("." + Name)
	if err != nil {
		panic(err)
	}
}

func AppConfig() depinject.Config {
	return depinject.Configs(
		appConfig,
		depinject.Supply(
			map[string]module.AppModuleBasic{
				genutiltypes.ModuleName: genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
			},
		),
	)
}

func New(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	var (
		app        = &App{}
		appBuilder *runtime.AppBuilder

		appConf = depinject.Configs(
			AppConfig(),
			depinject.Supply(appOpts, logger),
		)
	)

	if err := EnsureZeroMinGasPrices(appOpts); err != nil {
		panic(err)
	}

	var appModules map[string]appmodule.AppModule
	if err := depinject.Inject(appConf,
		&appBuilder,
		&appModules,
		&app.appCodec,
		&app.legacyAmino,
		&app.txConfig,
		&app.interfaceRegistry,
		&app.AuthKeeper,
		&app.BankKeeper,
		&app.StakingKeeper,
		&app.DistrKeeper,
		&app.DnsKeeper,
		&app.ReleaseKeeper,
		&app.GatewaysKeeper,
		&app.TokenomicsKeeper,
		&app.PqcKeeper,
	); err != nil {
		panic(err)
	}

	app.DnsKeeper.SetBankKeeper(app.BankKeeper)
	app.DnsKeeper.SetAccountKeeper(app.AuthKeeper)
	app.PqcKeeper.SetBankKeeper(app.BankKeeper)
	baseAppOptions = append(baseAppOptions, baseapp.SetOptimisticExecution())

	app.App = appBuilder.Build(db, traceStore, baseAppOptions...)
	app.SetAnteHandler(app.buildAnteHandler())
	app.SetPostHandler(app.buildPostHandler())

	app.sm = module.NewSimulationManagerFromAppModules(app.ModuleManager.Modules, make(map[string]module.AppModuleSimulation))
	app.sm.RegisterStoreDecoders()

	if err := app.Load(loadLatest); err != nil {
		panic(err)
	}

	app.AppOpts = appOpts

	return app
}

func (app *App) LegacyAmino() *codec.LegacyAmino                 { return app.legacyAmino }
func (app *App) AppCodec() codec.Codec                           { return app.appCodec }
func (app *App) InterfaceRegistry() codectypes.InterfaceRegistry { return app.interfaceRegistry }
func (app *App) TxConfig() client.TxConfig                       { return app.txConfig }

func (app *App) GetKey(storeKey string) *storetypes.KVStoreKey {
	kvStoreKey, ok := app.UnsafeFindStoreKey(storeKey).(*storetypes.KVStoreKey)
	if !ok {
		return nil
	}
	return kvStoreKey
}

func (app *App) SimulationManager() *module.SimulationManager { return app.sm }

func (app *App) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	app.App.RegisterAPIRoutes(apiSvr, apiConfig)
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
	docs.RegisterOpenAPIService(Name, apiSvr.Router)

}

func GetMaccPerms() map[string][]string {
	dup := make(map[string][]string)
	for _, perms := range moduleAccPerms {
		dup[perms.GetAccount()] = perms.GetPermissions()
	}
	return dup
}

func BlockedAddresses() map[string]bool {
	result := make(map[string]bool)
	if len(blockAccAddrs) > 0 {
		for _, addr := range blockAccAddrs {
			result[addr] = true
		}
	} else {
		for addr := range GetMaccPerms() {
			result[addr] = true
		}
	}
	return result
}

/* ---------- Ante chain ---------- */

func (app *App) buildAnteHandler() sdk.AnteHandler {
	ak := app.AuthKeeper
	bk := app.BankKeeper

	return sdk.ChainAnteDecorators(
		authante.NewSetUpContextDecorator(),
		NewEnforceZeroMinGasDecorator(),
		NewZeroFeeDecorator(),
		(&RateLimitDecorator{}).Init(ak).WithGatewaysKeeper(&app.GatewaysKeeper),
		authante.NewValidateBasicDecorator(),
		authante.NewTxTimeoutHeightDecorator(),
		authante.NewValidateMemoDecorator(ak),
		authante.NewConsumeGasForTxSizeDecorator(ak),
		NewMinSendDecorator(ak, app.TokenomicsKeeper),
		NewSendTaxDecorator(ak, app.TokenomicsKeeper),
		authante.NewDeductFeeDecorator(ak, bk, nil, nil),
		authante.NewSetPubKeyDecorator(ak),
		authante.NewSigVerificationDecorator(ak, app.txConfig.SignModeHandler()),
		NewPQCDualSignDecorator(app.Logger(), ak, app.PqcKeeper, app.appCodec),
		authante.NewIncrementSequenceDecorator(ak),
	)
}

func (app *App) buildPostHandler() sdk.PostHandler {
	return NewSendTaxPostHandler(app.AuthKeeper, app.BankKeeper, app.TokenomicsKeeper)
}
