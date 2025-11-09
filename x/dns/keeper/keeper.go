package keeper

import (
	"context"
	"fmt"

	"lumen/x/dns/types"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	authority    []byte

	Schema       collections.Schema
	Params       collections.Item[types.Params]
	Domain       collections.Map[string, types.Domain]
	Auction      collections.Map[string, types.Auction]
	OpsThisBlock collections.Item[uint64]

	bank types.BankKeeper

	dk types.DistrKeeper

	ak types.AccountKeeper
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
		Domain:       collections.NewMap(sb, types.DomainKey, "domain", collections.StringKey, codec.CollValue[types.Domain](cdc)),
		Auction:      collections.NewMap(sb, types.AuctionKey, "auction", collections.StringKey, codec.CollValue[types.Auction](cdc)),
		OpsThisBlock: collections.NewItem(sb, types.OpsThisBlockKey, "ops_this_block", collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

func (k Keeper) GetAuthority() []byte { return k.authority }

func (k Keeper) fqdn(domain, ext string) string {
	d, e := types.NormalizeDomainParts(domain, ext)
	return d + "." + e
}

func (k Keeper) nowSec(ctx context.Context) uint64 {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return uint64(sdkCtx.BlockTime().Unix())
}

func lifecycleStatus(now, expire, graceDays, auctionDays uint64) string {
	if expire == 0 {
		return "free"
	}
	if now < expire {
		return "active"
	}
	graceEnd := expire + graceDays*24*3600
	if now < graceEnd {
		return "grace"
	}
	auctionEnd := graceEnd + auctionDays*24*3600
	if now < auctionEnd {
		return "auction"
	}
	return "free"
}

func defaultDays(v, def uint64) uint64 {
	if v == 0 {
		return def
	}
	return v
}

func (k *Keeper) SetBankKeeper(b types.BankKeeper) { k.bank = b }

func (k *Keeper) SetAccountKeeper(ak types.AccountKeeper) { k.ak = ak }

func (k *Keeper) SetDistrKeeper(dk types.DistrKeeper) { k.dk = dk }
