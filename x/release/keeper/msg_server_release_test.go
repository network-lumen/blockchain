package keeper_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	coreaddress "cosmossdk.io/core/address"
	storetypes "cosmossdk.io/store/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkruntime "github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"

	"lumen/app"
	"lumen/x/release/keeper"
	"lumen/x/release/types"
)

type releaseFixture struct {
	ctx       sdk.Context
	keeper    keeper.Keeper
	msgSrv    types.MsgServer
	authority string
	addressBz []byte
	addrCodec coreaddress.Codec
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	memKey := storetypes.NewTransientStoreKey("release_msg_server_test")
	ctx := testutil.DefaultContextWithDB(t, storeKey, memKey).Ctx
	ctx = ctx.WithBlockTime(time.Unix(1_000, 0))

	ir := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	addrCodec := authcodec.NewBech32Codec(app.AccountAddressPrefix)
	authorityBz := sdk.AccAddress(bytes.Repeat([]byte{0x42}, 20))
	authorityStr, err := addrCodec.BytesToString(authorityBz)
	require.NoError(t, err)

	k := keeper.NewKeeper(sdkruntime.NewKVStoreService(storeKey), cdc, addrCodec, authorityBz)
	require.NoError(t, k.SetParams(ctx, types.DefaultParams()))

	return &releaseFixture{
		ctx:       ctx,
		keeper:    k,
		msgSrv:    keeper.NewMsgServerImpl(k),
		authority: authorityStr,
		addressBz: authorityBz,
		addrCodec: addrCodec,
	}
}

func (f *releaseFixture) storeRelease(t *testing.T, id uint64) {
	t.Helper()
	release := types.Release{
		Id:        id,
		Version:   fmt.Sprintf("1.0.%d", id),
		Channel:   "beta",
		Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("a", 64), Urls: []string{"https://example.com/bin"}}},
		Status:    types.Release_PENDING,
	}
	require.NoError(t, f.keeper.Release.Set(f.ctx, id, release))
}

func TestMsgServerValidateRelease(t *testing.T) {
	f := newReleaseFixture(t)
	f.storeRelease(t, 1)

	_, err := f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{
		Authority: f.authority,
		Id:        1,
	})
	require.NoError(t, err)

	release, err := f.keeper.Release.Get(f.ctx, 1)
	require.NoError(t, err)
	require.Equal(t, types.Release_VALIDATED, release.Status)

	// idempotent
	_, err = f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{
		Authority: f.authority,
		Id:        1,
	})
	require.NoError(t, err)
}

func TestMsgServerValidateReleaseUnauthorized(t *testing.T) {
	f := newReleaseFixture(t)
	f.storeRelease(t, 1)

	other := sdk.AccAddress(bytes.Repeat([]byte{0x01}, 20)).String()
	_, err := f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{
		Authority: other,
		Id:        1,
	})
	require.Error(t, err)
}

func TestMsgServerRejectRelease(t *testing.T) {
	f := newReleaseFixture(t)
	f.storeRelease(t, 1)

	_, err := f.msgSrv.RejectRelease(f.ctx, &types.MsgRejectRelease{
		Authority: f.authority,
		Id:        1,
	})
	require.NoError(t, err)

	release, err := f.keeper.Release.Get(f.ctx, 1)
	require.NoError(t, err)
	require.Equal(t, types.Release_REJECTED, release.Status)
	require.False(t, release.EmergencyOk)
	require.Zero(t, release.EmergencyUntil)

	other := sdk.AccAddress(bytes.Repeat([]byte{0x02}, 20)).String()
	_, err = f.msgSrv.RejectRelease(f.ctx, &types.MsgRejectRelease{
		Authority: other,
		Id:        1,
	})
	require.Error(t, err)
}

func TestMsgServerSetEmergency(t *testing.T) {
	f := newReleaseFixture(t)
	f.storeRelease(t, 1)

	_, err := f.msgSrv.SetEmergency(f.ctx, &types.MsgSetEmergency{
		Creator:      f.authority,
		Id:           1,
		EmergencyOk:  true,
		EmergencyTtl: 30,
	})
	require.NoError(t, err)

	release, err := f.keeper.Release.Get(f.ctx, 1)
	require.NoError(t, err)
	require.True(t, release.EmergencyOk)
	require.Equal(t, f.ctx.BlockTime().Unix()+30, release.EmergencyUntil)

	_, err = f.msgSrv.SetEmergency(f.ctx, &types.MsgSetEmergency{
		Creator:     f.authority,
		Id:          1,
		EmergencyOk: false,
	})
	require.NoError(t, err)

	release, err = f.keeper.Release.Get(f.ctx, 1)
	require.NoError(t, err)
	require.False(t, release.EmergencyOk)
	require.Zero(t, release.EmergencyUntil)

	other := sdk.AccAddress(bytes.Repeat([]byte{0x03}, 20)).String()
	_, err = f.msgSrv.SetEmergency(f.ctx, &types.MsgSetEmergency{
		Creator:     other,
		Id:          1,
		EmergencyOk: true,
	})
	require.Error(t, err)
}
