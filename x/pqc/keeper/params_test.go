package keeper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkruntime "github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/x/pqc/types"
)

func TestParamsForceRequired(t *testing.T) {
	ctx, keeper := newKeeperForParamsTest(t)

	params := keeper.GetParams(ctx)
	require.Equal(t, types.PqcPolicy_PQC_POLICY_REQUIRED, params.Policy)

	require.Panics(t, func() {
		_ = keeper.SetParams(ctx, types.Params{Policy: types.PqcPolicy_PQC_POLICY_OPTIONAL})
	})
}

func newKeeperForParamsTest(t *testing.T) (sdk.Context, Keeper) {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	memKey := storetypes.NewTransientStoreKey("transient_pqc_params_test")
	sdkCtx := testutil.DefaultContextWithDB(t, storeKey, memKey).Ctx
	sdkCtx = sdkCtx.WithBlockTime(time.Unix(0, 0))

	ir := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	k := NewKeeper(sdkruntime.NewKVStoreService(storeKey), cdc)
	require.NoError(t, k.SetParams(sdkCtx, types.DefaultParams()))

	return sdkCtx, k
}
