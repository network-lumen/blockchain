package tokenomics

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
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stTypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"lumen/x/tokenomics/keeper"
	"lumen/x/tokenomics/types"
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

	BankKeeper         bankkeeper.Keeper
	StakingKeeper      *stakingkeeper.Keeper
	DistributionKeeper distrkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	TokenomicsKeeper keeper.Keeper
	Module           appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority)
	k.SetBankKeeper(bankAdapter{bk: in.BankKeeper})
	if in.StakingKeeper != nil {
		k.SetStakingKeeper(stakingAdapter{sk: in.StakingKeeper})
	}
	k.SetDistributionKeeper(distributionAdapter{dk: in.DistributionKeeper})
	m := NewAppModule(in.Cdc, k)
	return ModuleOutputs{TokenomicsKeeper: k, Module: m}
}

type bankAdapter struct{ bk bankkeeper.Keeper }

func (b bankAdapter) MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error {
	return b.bk.MintCoins(ctx, moduleName, amt)
}

func (b bankAdapter) SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	return b.bk.SendCoinsFromModuleToModule(ctx, senderModule, recipientModule, amt)
}

type distributionAdapter struct{ dk distrkeeper.Keeper }

func (d distributionAdapter) WithdrawValidatorCommission(ctx context.Context, valAddr sdk.ValAddress) (sdk.Coins, error) {
	return d.dk.WithdrawValidatorCommission(ctx, valAddr)
}

type stakingAdapter struct{ sk *stakingkeeper.Keeper }

func (s stakingAdapter) IterateValidators(ctx context.Context, fn func(index int64, validator stTypes.ValidatorI) (stop bool)) {
	_ = s.sk.IterateValidators(ctx, fn)
}

func (s stakingAdapter) ValidatorAddressCodec() address.Codec {
	return s.sk.ValidatorAddressCodec()
}
