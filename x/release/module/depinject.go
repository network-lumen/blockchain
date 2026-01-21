package release

import (
	"context"

	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"

	"lumen/x/release/keeper"
	"lumen/x/release/types"
)

var _ depinject.OnePerModuleType = AppModule{}

func (AppModule) IsOnePerModuleType() {}

func init() {
	appconfig.Register(
		&types.Module{},
		appconfig.Provide(ProvideModule),
	)
}

type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	AuthKeeper         types.AuthKeeper
	BankKeeper         bankkeeper.Keeper
	DistributionKeeper distrkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	ReleaseKeeper keeper.Keeper
	Module        appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}
	k := keeper.NewKeeper(
		in.StoreService,
		in.Cdc,
		in.AddressCodec,
		authority,
	)
	k.SetBankKeeper(bankAdapter{bk: in.BankKeeper})
	k.SetDistributionKeeper(distributionAdapter{dk: in.DistributionKeeper})
	m := NewAppModule(in.Cdc, k, in.AuthKeeper, bankAdapter{bk: in.BankKeeper})

	return ModuleOutputs{ReleaseKeeper: k, Module: m}
}

type bankAdapter struct{ bk bankkeeper.Keeper }

// Implement the subset used by this module
func (b bankAdapter) SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	return b.bk.SpendableCoins(ctx, addr)
}
func (b bankAdapter) SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, module string, amt sdk.Coins) error {
	return b.bk.SendCoinsFromAccountToModule(ctx, sender, module, amt)
}
func (b bankAdapter) SendCoinsFromModuleToAccount(ctx context.Context, module string, rcpt sdk.AccAddress, amt sdk.Coins) error {
	return b.bk.SendCoinsFromModuleToAccount(ctx, module, rcpt, amt)
}
func (b bankAdapter) SendCoinsFromModuleToModule(ctx context.Context, from, to string, amt sdk.Coins) error {
	return b.bk.SendCoinsFromModuleToModule(ctx, from, to, amt)
}
func (b bankAdapter) SendCoins(ctx context.Context, from, to sdk.AccAddress, amt sdk.Coins) error {
	return b.bk.SendCoins(ctx, from, to, amt)
}

type distributionAdapter struct{ dk distrkeeper.Keeper }

func (d distributionAdapter) FundCommunityPool(ctx context.Context, amount sdk.Coins, depositor sdk.AccAddress) error {
	return d.dk.FundCommunityPool(ctx, amount, depositor)
}
