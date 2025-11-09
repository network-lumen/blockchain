package gateways

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"lumen/x/gateways/keeper"
	"lumen/x/gateways/types"
)

var _ depinject.OnePerModuleType = AppModule{}

func (AppModule) IsOnePerModuleType() {}

func init() {
	appconfig.Register(&types.Module{}, appconfig.Provide(ProvideModule))
}

type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	AuthKeeper       types.AuthKeeper
	BankKeeper       types.BankKeeper
	TokenomicsKeeper types.TokenomicsKeeper
}

type ModuleOutputs struct {
	depinject.Out

	GatewaysKeeper keeper.Keeper
	Module         appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}
	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority)
	k.SetBankKeeper(in.BankKeeper)
	k.SetAccountKeeper(in.AuthKeeper)
	k.SetTokenomicsKeeper(in.TokenomicsKeeper)
	m := NewAppModule(in.Cdc, k, in.AuthKeeper, in.BankKeeper)
	return ModuleOutputs{GatewaysKeeper: k, Module: m}
}
