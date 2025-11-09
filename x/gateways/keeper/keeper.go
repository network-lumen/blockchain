package keeper

import (
	"context"
	"fmt"
	"math/bits"
	"strings"

	"lumen/app/denom"
	"lumen/x/gateways/types"

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
	Gateways    collections.Map[uint64, types.Gateway]
	GatewaySeq  collections.Sequence
	Contracts   collections.Map[uint64, types.Contract]
	ContractSeq collections.Sequence

	bank       types.BankKeeper
	ak         types.AccountKeeper
	tokenomics types.TokenomicsKeeper
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

		Params:      collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		Gateways:    collections.NewMap(sb, types.GatewayKey, "gateway", collections.Uint64Key, codec.CollValue[types.Gateway](cdc)),
		GatewaySeq:  collections.NewSequence(sb, types.GatewaySeqKey, "gateway_seq"),
		Contracts:   collections.NewMap(sb, types.ContractKey, "contract", collections.Uint64Key, codec.CollValue[types.Contract](cdc)),
		ContractSeq: collections.NewSequence(sb, types.ContractSeqKey, "contract_seq"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k *Keeper) SetBankKeeper(b types.BankKeeper)        { k.bank = b }
func (k *Keeper) SetAccountKeeper(ak types.AccountKeeper) { k.ak = ak }
func (k *Keeper) SetTokenomicsKeeper(tk types.TokenomicsKeeper) {
	k.tokenomics = tk
}

func (k Keeper) GetAuthority() []byte { return k.authority }

func (k Keeper) GetParams(ctx context.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return types.DefaultParams()
	}
	return params
}

func (k Keeper) SetParams(ctx context.Context, params types.Params) error {
	return k.Params.Set(ctx, params)
}

func (k Keeper) nextGatewayID(ctx context.Context) (uint64, error) {
	return k.GatewaySeq.Next(ctx)
}

func (k Keeper) nextContractID(ctx context.Context) (uint64, error) {
	return k.ContractSeq.Next(ctx)
}

func (k Keeper) now(ctx context.Context) sdk.Context {
	return sdk.UnwrapSDKContext(ctx)
}

func (k Keeper) nowUnix(ctx context.Context) int64 {
	return k.now(ctx).BlockTime().Unix()
}

func (k Keeper) mustAddress(bech32 string) (sdk.AccAddress, error) {
	if k.ak == nil {
		return nil, fmt.Errorf("account keeper not set")
	}
	bz, err := k.ak.AddressCodec().StringToBytes(strings.TrimSpace(bech32))
	if err != nil {
		return nil, err
	}
	return sdk.AccAddress(bz), nil
}

func (k Keeper) gatewayByID(ctx context.Context, id uint64) (types.Gateway, error) {
	gateway, err := k.Gateways.Get(ctx, id)
	if err != nil {
		return types.Gateway{}, types.ErrNotFound
	}
	return gateway, nil
}

func (k Keeper) contractByID(ctx context.Context, id uint64) (types.Contract, error) {
	contract, err := k.Contracts.Get(ctx, id)
	if err != nil {
		return types.Contract{}, types.ErrNotFound
	}
	return contract, nil
}

func (k Keeper) setGateway(ctx context.Context, gateway types.Gateway) error {
	return k.Gateways.Set(ctx, gateway.Id, gateway)
}

func (k Keeper) setContract(ctx context.Context, contract types.Contract) error {
	return k.Contracts.Set(ctx, contract.Id, contract)
}

func (k Keeper) payFromModule(ctx context.Context, module string, to sdk.AccAddress, amount sdkmath.Int) error {
	if !amount.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, amount))
	return k.bank.SendCoinsFromModuleToAccount(ctx, module, to, coins)
}

func (k Keeper) moveToModule(ctx context.Context, from sdk.AccAddress, module string, amount sdkmath.Int) error {
	if !amount.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, amount))
	return k.bank.SendCoinsFromAccountToModule(ctx, from, module, coins)
}

func (k Keeper) moveModuleToModule(ctx context.Context, fromModule, toModule string, amount sdkmath.Int) error {
	if !amount.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, amount))
	return k.bank.SendCoinsFromModuleToModule(ctx, fromModule, toModule, coins)
}

func (k Keeper) collectGatewayFee(ctx context.Context, payer string, amount uint64) error {
	if amount == 0 || k.bank == nil {
		return nil
	}
	fee := sdkmath.NewIntFromUint64(amount)
	if !fee.IsPositive() {
		return nil
	}
	addr, err := k.mustAddress(payer)
	if err != nil {
		return err
	}
	return k.moveToModule(ctx, addr, authtypes.FeeCollectorName, fee)
}

func (k Keeper) collectActionFee(ctx context.Context, payer string) error {
	params := k.GetParams(ctx)
	return k.collectGatewayFee(ctx, payer, params.ActionFeeUlmn)
}

func (k Keeper) collectRegisterFee(ctx context.Context, payer string) error {
	params := k.GetParams(ctx)
	return k.collectGatewayFee(ctx, payer, params.RegisterGatewayFeeUlmn)
}

func (k Keeper) applyCommission(amount sdkmath.Int, bps uint32) sdkmath.Int {
	if bps == 0 {
		return sdkmath.ZeroInt()
	}
	return amount.MulRaw(int64(bps)).QuoRaw(10_000)
}

func (k Keeper) safeAmountFromString(raw string) sdkmath.Int {
	if raw == "" {
		return sdkmath.ZeroInt()
	}
	val, ok := sdkmath.NewIntFromString(raw)
	if !ok {
		return sdkmath.ZeroInt()
	}
	return val
}

func (k Keeper) safeAddUint64(a, b uint64) (uint64, error) {
	sum, carry := bits.Add64(a, b, 0)
	if carry != 0 {
		return 0, types.ErrOverflow
	}
	return sum, nil
}

func (k Keeper) safeMulUint64(a, b uint64) (uint64, error) {
	hi, lo := bits.Mul64(a, b)
	if hi != 0 {
		return 0, types.ErrOverflow
	}
	return lo, nil
}

func (k Keeper) contractOffsetTime(contract types.Contract, months uint64, monthSeconds uint64) (uint64, error) {
	offset, err := k.safeMulUint64(months, monthSeconds)
	if err != nil {
		return 0, err
	}
	return k.safeAddUint64(contract.StartTime, offset)
}
