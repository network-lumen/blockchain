package keeper

import (
	"context"
	"fmt"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/x/release/types"
)

type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	authority    []byte

	Schema collections.Schema
	Params collections.Item[types.Params]

	Release    collections.Map[uint64, types.Release]
	ReleaseSeq collections.Sequence
	ByVersion  collections.Map[string, uint64]
	ByTriple   collections.Map[string, uint64]

	bank  types.BankKeeper
	distr types.DistributionKeeper

	// Hardening state.
	StateVersion collections.Item[uint64]
	ExpiryTTL    collections.Item[uint64]

	EscrowAmount    collections.Map[uint64, uint64]
	EscrowPublisher collections.Map[uint64, string]

	ExpiryByID  collections.Map[uint64, int64]
	ExpiryQueue collections.Map[collections.Pair[int64, uint64], bool]
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

		Params:     collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		Release:    collections.NewMap(sb, types.ReleaseKey, "release", collections.Uint64Key, codec.CollValue[types.Release](cdc)),
		ReleaseSeq: collections.NewSequence(sb, types.ReleaseSeqKey, "release_seq"),
		ByVersion:  collections.NewMap(sb, types.ByVersionKey, "by_version", collections.StringKey, collections.Uint64Value),
		ByTriple:   collections.NewMap(sb, types.ByTripleKey, "by_cpk", collections.StringKey, collections.Uint64Value),

		StateVersion: collections.NewItem(sb, types.StateVersionKey, "state_version", collections.Uint64Value),
		ExpiryTTL:    collections.NewItem(sb, types.ExpiryTTLKey, "expiry_ttl", collections.Uint64Value),

		EscrowAmount:    collections.NewMap(sb, types.EscrowAmountKey, "escrow_amount", collections.Uint64Key, collections.Uint64Value),
		EscrowPublisher: collections.NewMap(sb, types.EscrowPublisherKey, "escrow_publisher", collections.Uint64Key, collections.StringValue),

		ExpiryByID: collections.NewMap(sb, types.ExpiryByIDKey, "expiry_by_id", collections.Uint64Key, collections.Int64Value),
		ExpiryQueue: collections.NewMap(
			sb,
			types.ExpiryQueueKey,
			"expiry_queue",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key),
			collections.BoolValue,
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() []byte { return k.authority }

func (k Keeper) GetParams(ctx context.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return types.DefaultParams()
	}
	return p
}

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	return k.Params.Set(ctx, p)
}

func tripleKey(channel, platform, kind string) string {
	return strings.ToLower(channel) + "|" + strings.ToLower(platform) + "|" + strings.ToLower(kind)
}

func (k Keeper) nowUnix(ctx context.Context) int64 {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return sdkCtx.BlockTime().Unix()
}

func (k *Keeper) SetBankKeeper(b types.BankKeeper) { k.bank = b }

func (k *Keeper) SetDistributionKeeper(d types.DistributionKeeper) { k.distr = d }
