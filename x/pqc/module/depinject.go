package pqc

import (
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"

	"lumen/x/pqc/keeper"
	"lumen/x/pqc/types"
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
	BankKeeper   types.BankKeeper
}

type ModuleOutputs struct {
	depinject.Out

	PqcKeeper keeper.Keeper
	Module    appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	k := keeper.NewKeeper(in.StoreService, in.Cdc)
	k.SetBankKeeper(in.BankKeeper)
	m := NewAppModule(in.Cdc, k)
	return ModuleOutputs{PqcKeeper: k, Module: m}
}
