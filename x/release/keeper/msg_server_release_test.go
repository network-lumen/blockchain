package keeper_test

import (
	"bytes"
	"context"
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
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

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
	bank      *bankMock
	distr     *distributionMock
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
	bank := newBankMock()
	distr := &distributionMock{bank: bank}
	k.SetBankKeeper(bank)
	k.SetDistributionKeeper(distr)

	return &releaseFixture{
		ctx:       ctx,
		keeper:    k,
		msgSrv:    keeper.NewMsgServerImpl(k),
		authority: authorityStr,
		addressBz: authorityBz,
		addrCodec: addrCodec,
		bank:      bank,
		distr:     distr,
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
		CreatedAt: f.ctx.BlockTime().Unix(),
	}
	require.NoError(t, f.keeper.Release.Set(f.ctx, id, release))
}

type bankMock struct {
	balances map[string]sdk.Coins
}

func newBankMock() *bankMock { return &bankMock{balances: map[string]sdk.Coins{}} }

func (b *bankMock) SpendableCoins(_ context.Context, addr sdk.AccAddress) sdk.Coins {
	return b.balances[addr.String()]
}

func (b *bankMock) SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error {
	return b.send(ctx, fromAddr.String(), toAddr.String(), amt)
}

func (b *bankMock) SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	return b.send(ctx, senderAddr.String(), moduleKey(recipientModule), amt)
}

func (b *bankMock) SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	return b.send(ctx, moduleKey(senderModule), recipientAddr.String(), amt)
}

func (b *bankMock) SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	return b.send(ctx, moduleKey(senderModule), moduleKey(recipientModule), amt)
}

func moduleKey(module string) string { return authtypes.NewModuleAddress(module).String() }

func (b *bankMock) send(_ context.Context, fromKey, toKey string, amt sdk.Coins) error {
	if amt.IsZero() {
		return nil
	}
	fromBal := b.balances[fromKey]
	if !fromBal.IsAllGTE(amt) {
		return fmt.Errorf("insufficient funds: %s < %s", fromBal, amt)
	}
	b.balances[fromKey] = fromBal.Sub(amt...)
	b.balances[toKey] = b.balances[toKey].Add(amt...)
	return nil
}

func (b *bankMock) setBalance(addr string, coins sdk.Coins) { b.balances[addr] = coins }

type distributionMock struct {
	bank          *bankMock
	communityPool sdk.Coins
}

func (d *distributionMock) FundCommunityPool(_ context.Context, amount sdk.Coins, depositor sdk.AccAddress) error {
	// Simulate a real community pool credit by removing funds from the depositor
	// and tracking them in the fee pool accumulator.
	if amount.IsZero() {
		return nil
	}
	if err := d.bank.send(context.Background(), depositor.String(), "community_pool", amount); err != nil {
		return err
	}
	d.communityPool = d.communityPool.Add(amount...)
	return nil
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

	// PENDING-only: second validation must fail.
	_, err = f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{
		Authority: f.authority,
		Id:        1,
	})
	require.Error(t, err)
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

	// PENDING-only: second rejection must fail.
	_, err = f.msgSrv.RejectRelease(f.ctx, &types.MsgRejectRelease{
		Authority: f.authority,
		Id:        1,
	})
	require.Error(t, err)

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
	require.Error(t, err)
}
