package keeper

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/crypto/pqc/dilithium"
	"lumen/x/pqc/types"
)

type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	scheme       dilithium.Scheme

	Schema   collections.Schema
	Params   collections.Item[types.Params]
	Accounts collections.Map[string, types.AccountPQC]

	bank types.BankKeeper
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		scheme:       dilithium.Default(),
		Params:       collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		Accounts:     collections.NewMap(sb, types.AccountKeyPrefix, "account_pqc", collections.StringKey, codec.CollValue[types.AccountPQC](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) Scheme() dilithium.Scheme {
	return k.scheme
}

func (k *Keeper) SetBankKeeper(bank types.BankKeeper) {
	k.bank = bank
}

func (k Keeper) BankKeeper() types.BankKeeper {
	return k.bank
}

func (k Keeper) GetParams(ctx context.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		params = types.DefaultParams()
	}
	params.Policy = types.PqcPolicy_PQC_POLICY_REQUIRED
	return params
}

func (k Keeper) SetParams(ctx context.Context, params types.Params) error {
	if params.Policy != types.PqcPolicy_PQC_POLICY_REQUIRED {
		panic("pqc: SetParams refused: policy must be REQUIRED")
	}
	if err := params.Validate(); err != nil {
		return err
	}
	params.Policy = types.PqcPolicy_PQC_POLICY_REQUIRED
	return k.Params.Set(ctx, params)
}

func (k Keeper) SetAccountPQC(ctx context.Context, addr sdk.AccAddress, info types.AccountPQC) error {
	info.Addr = addr.String()
	if len(info.PubKeyHash) != sha256.Size {
		return fmt.Errorf("pub_key_hash must be %d bytes", sha256.Size)
	}
	info.PubKeyHash = append([]byte(nil), info.PubKeyHash...)
	return k.Accounts.Set(ctx, info.Addr, info)
}

func (k Keeper) GetAccountPQC(ctx context.Context, addr sdk.AccAddress) (types.AccountPQC, bool, error) {
	info, err := k.Accounts.Get(ctx, addr.String())
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.AccountPQC{}, false, nil
		}
		return types.AccountPQC{}, false, err
	}
	normalized, err := k.ensureAccountHash(ctx, addr, info)
	if err != nil {
		return types.AccountPQC{}, false, err
	}
	return normalized, true, nil
}

func (k Keeper) HasAccountPQC(ctx context.Context, addr sdk.AccAddress) (bool, error) {
	_, found, err := k.GetAccountPQC(ctx, addr)
	return found, err
}

func (k Keeper) ensureAccountHash(ctx context.Context, addr sdk.AccAddress, info types.AccountPQC) (types.AccountPQC, error) {
	if len(info.PubKeyHash) == sha256.Size {
		info.PubKeyHash = append([]byte(nil), info.PubKeyHash...)
		return info, nil
	}
	if len(info.PubKeyHash) == 0 {
		return info, nil
	}
	hash := sha256.Sum256(info.PubKeyHash)
	info.PubKeyHash = append([]byte(nil), hash[:]...)
	if err := k.Accounts.Set(ctx, addr.String(), info); err != nil {
		return types.AccountPQC{}, err
	}
	return info, nil
}
