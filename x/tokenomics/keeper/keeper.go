package keeper

import (
	"context"
	"fmt"

	"lumen/x/tokenomics/types"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	authority    []byte

	Schema      collections.Schema
	Params      collections.Item[types.Params]
	TotalMinted collections.Item[string]

	bank     types.BankKeeper
	distr    types.DistributionKeeper
	staking  types.StakingKeeper
	slashing types.SlashingKeeper
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		Params:       collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		TotalMinted:  collections.NewItem(sb, types.TotalMintedKey, "total_minted_ulmn", collections.StringValue),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

func (k Keeper) GetAuthority() []byte {
	return k.authority
}

func (k Keeper) GetParams(ctx context.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		def := types.DefaultParams()
		return def
	}
	return params
}

func (k Keeper) SetParams(ctx context.Context, params types.Params) error {
	return k.Params.Set(ctx, params)
}

func (k *Keeper) SetBankKeeper(b types.BankKeeper) {
	k.bank = b
}

func (k *Keeper) SetDistributionKeeper(d types.DistributionKeeper) {
	k.distr = d
}

func (k *Keeper) SetStakingKeeper(s types.StakingKeeper) {
	k.staking = s
}

func (k *Keeper) SetSlashingKeeper(s types.SlashingKeeper) {
	if s == nil {
		return
	}
	k.slashing = s
}

func (k Keeper) GetTotalMintedUlmn(ctx context.Context) sdkmath.Int {
	stored, err := k.TotalMinted.Get(ctx)
	if err != nil {
		return sdkmath.ZeroInt()
	}
	amt, ok := sdkmath.NewIntFromString(stored)
	if !ok {
		panic(fmt.Errorf("invalid total minted amount: %s", stored))
	}
	return amt
}

func (k Keeper) SetTotalMintedUlmn(ctx context.Context, amt sdkmath.Int) error {
	if amt.IsNegative() {
		return fmt.Errorf("total minted cannot be negative")
	}
	return k.TotalMinted.Set(ctx, amt.String())
}

func (k Keeper) HasBankKeeper() bool {
	return k.bank != nil
}

func (k Keeper) HasDistributionKeeper() bool {
	return k.distr != nil
}

func (k Keeper) HasStakingKeeper() bool {
	return k.staking != nil
}

func (k Keeper) HasSlashingKeeper() bool {
	return k.slashing != nil
}

func (k Keeper) SlashingKeeper() types.SlashingKeeper {
	return k.slashing
}

func (k Keeper) MintToFeeCollector(ctx sdk.Context, coins sdk.Coins) {
	if !k.HasBankKeeper() {
		panic("tokenomics bank keeper is not set")
	}
	if err := k.bank.MintCoins(ctx, types.ModuleName, coins); err != nil {
		panic(err)
	}
	if err := k.bank.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, coins); err != nil {
		panic(err)
	}
}

func (k Keeper) MintToCommunityPool(ctx sdk.Context, coins sdk.Coins) {
	if !k.HasBankKeeper() || !k.HasDistributionKeeper() {
		return
	}
	if coins.Empty() {
		return
	}
	if err := k.bank.MintCoins(ctx, types.ModuleName, coins); err != nil {
		panic(err)
	}
	depositor := authtypes.NewModuleAddress(types.ModuleName)
	if err := k.distr.FundCommunityPool(ctx, coins, depositor); err != nil {
		panic(err)
	}
}
