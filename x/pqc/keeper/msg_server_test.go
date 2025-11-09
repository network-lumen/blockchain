package keeper_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkruntime "github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/crypto/pqc/dilithium"
	pqckeeper "lumen/x/pqc/keeper"
	"lumen/x/pqc/types"
)

type fixture struct {
	ctx    sdk.Context
	keeper pqckeeper.Keeper
	msgSrv types.MsgServer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	memKey := storetypes.NewTransientStoreKey("transient_pqc_test")
	ctx := testutil.DefaultContextWithDB(t, storeKey, memKey).Ctx
	ctx = ctx.WithBlockTime(time.Unix(100, 0))

	ir := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	keeper := pqckeeper.NewKeeper(sdkruntime.NewKVStoreService(storeKey), cdc)
	require.NoError(t, keeper.SetParams(ctx, types.DefaultParams()))

	return &fixture{
		ctx:    ctx,
		keeper: keeper,
		msgSrv: pqckeeper.NewMsgServerImpl(keeper),
	}
}

func (f *fixture) newMessage(t *testing.T) (*types.MsgLinkAccountPQC, sdk.AccAddress) {
	t.Helper()

	addr := sdk.AccAddress(bytes.Repeat([]byte{0x01}, 20))
	scheme := dilithium.Default()
	pub, _, err := scheme.GenerateKey(bytes.Repeat([]byte{0xAB}, 32))
	require.NoError(t, err)

	return types.NewMsgLinkAccountPQC(addr, scheme.Name(), pub), addr
}

func TestLinkAccountPQC_SingleLink(t *testing.T) {
	f := newFixture(t)
	msg, addr := f.newMessage(t)

	_, err := f.msgSrv.LinkAccountPQC(f.ctx, msg)
	require.NoError(t, err)

	account, found, err := f.keeper.GetAccountPQC(f.ctx, addr)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, msg.Scheme, account.Scheme)
	require.Equal(t, msg.PubKey, account.PubKey)
	require.Equal(t, f.ctx.BlockTime().Unix(), account.AddedAt)

	// rotation disabled: attempting to relink must fail
	_, err = f.msgSrv.LinkAccountPQC(f.ctx, msg)
	require.ErrorIs(t, err, types.ErrAccountRotationDisabled)
}

func TestLinkAccountPQC_RotationAllowed(t *testing.T) {
	f := newFixture(t)
	params := types.DefaultParams()
	params.AllowAccountRotate = true
	require.NoError(t, f.keeper.SetParams(f.ctx, params))

	msg, addr := f.newMessage(t)
	_, err := f.msgSrv.LinkAccountPQC(f.ctx, msg)
	require.NoError(t, err)

	scheme := dilithium.Default()
	newPub, _, err := scheme.GenerateKey(bytes.Repeat([]byte{0xCD}, 32))
	require.NoError(t, err)

	rotate := types.NewMsgLinkAccountPQC(addr, scheme.Name(), newPub)
	_, err = f.msgSrv.LinkAccountPQC(f.ctx, rotate)
	require.NoError(t, err)

	account, found, err := f.keeper.GetAccountPQC(f.ctx, addr)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []byte(newPub), account.PubKey)
}

func TestQueryAccount(t *testing.T) {
	f := newFixture(t)
	msg, addr := f.newMessage(t)

	_, err := f.msgSrv.LinkAccountPQC(f.ctx, msg)
	require.NoError(t, err)

	query := pqckeeper.NewQueryServerImpl(f.keeper)
	resp, err := query.AccountPQC(f.ctx, &types.QueryAccountPQCRequest{Addr: addr.String()})
	require.NoError(t, err)
	require.Equal(t, msg.PubKey, resp.Account.PubKey)

	_, err = query.AccountPQC(f.ctx, &types.QueryAccountPQCRequest{Addr: sdk.AccAddress(bytes.Repeat([]byte{0x02}, 20)).String()})
	require.Error(t, err)
}

func TestGenesisRoundTrip(t *testing.T) {
	f := newFixture(t)

	addr := sdk.AccAddress(bytes.Repeat([]byte{0x0A}, 20))
	scheme := dilithium.Default()
	pub, _, err := scheme.GenerateKey(bytes.Repeat([]byte{0xEF}, 32))
	require.NoError(t, err)

	genesis := types.GenesisState{
		Params: types.DefaultParams(),
		Accounts: []types.AccountPQC{
			{
				Addr:    addr.String(),
				Scheme:  scheme.Name(),
				PubKey:  pub,
				AddedAt: 42,
			},
		},
	}

	require.NoError(t, f.keeper.InitGenesis(f.ctx, genesis))

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.Equal(t, genesis.Params.Policy, exported.Params.Policy)
	require.Equal(t, genesis.Params.MinScheme, exported.Params.MinScheme)
	require.Equal(t, genesis.Params.AllowAccountRotate, exported.Params.AllowAccountRotate)
	require.Equal(t, genesis.Accounts, exported.Accounts)
}
