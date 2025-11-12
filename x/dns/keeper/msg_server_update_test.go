package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"lumen/app/denom"
	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func TestMsgUpdateChargesFixedFee(t *testing.T) {
	f := initFixture(t)
	bank := newMockBankKeeper()
	f.keeper.SetBankKeeper(bank)

	const updateFee = 5_000
	setUpdateFee(t, f, updateFee)

	ownerAddr := sdk.AccAddress([]byte("owner________________"))
	owner, err := f.addressCodec.BytesToString(ownerAddr)
	require.NoError(t, err)

	fee := sdkmath.NewIntFromUint64(updateFee)
	bank.setAccount(ownerAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, fee.MulRaw(2))))
	bank.setModule(authtypes.FeeCollectorName, sdk.NewCoins())

	name := "example.lumen"
	err = f.keeper.Domain.Set(f.ctx, name, types.Domain{
		Index: name,
		Name:  name,
		Owner: owner,
		Records: []*types.Record{
			{Key: "cid", Value: "old", Ttl: 0},
		},
	})
	require.NoError(t, err)

	msg := &types.MsgUpdate{
		Creator: owner,
		Domain:  "example",
		Ext:     "lumen",
		Records: []*types.Record{
			{Key: "cid", Value: "new", Ttl: 0},
		},
	}

	_, err = keeper.NewMsgServerImpl(f.keeper).Update(f.ctx, msg)
	require.NoError(t, err)

	dom, err := f.keeper.Domain.Get(f.ctx, name)
	require.NoError(t, err)
	require.Equal(t, msg.Records, dom.Records)

	collected := bank.modules[authtypes.FeeCollectorName]
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, fee)), collected)

	requireEventAttrs(t, f, "example.lumen", fee.String())
}

func TestMsgUpdateFailsOnInsufficientFunds(t *testing.T) {
	f := initFixture(t)
	bank := newMockBankKeeper()
	f.keeper.SetBankKeeper(bank)

	setUpdateFee(t, f, 1_000_000)

	ownerAddr := sdk.AccAddress([]byte("owner________________"))
	owner, err := f.addressCodec.BytesToString(ownerAddr)
	require.NoError(t, err)

	name := "example.lumen"
	originalRecords := []*types.Record{{Key: "cid", Value: "old", Ttl: 0}}
	err = f.keeper.Domain.Set(f.ctx, name, types.Domain{
		Index:   name,
		Name:    name,
		Owner:   owner,
		Records: originalRecords,
	})
	require.NoError(t, err)

	msg := &types.MsgUpdate{
		Creator: owner,
		Domain:  "example",
		Ext:     "lumen",
		Records: []*types.Record{{Key: "cid", Value: "new", Ttl: 0}},
	}

	sdkCtx := sdk.UnwrapSDKContext(f.ctx)
	prevEvents := len(sdkCtx.EventManager().Events())

	_, err = keeper.NewMsgServerImpl(f.keeper).Update(f.ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient funds")

	dom, err := f.keeper.Domain.Get(f.ctx, name)
	require.NoError(t, err)
	require.Equal(t, originalRecords, dom.Records, "domain records must remain unchanged")

	require.Nil(t, bank.modules[authtypes.FeeCollectorName])
	require.Equal(t, 0, len(bank.accounts))

	require.Equal(t, prevEvents, len(sdkCtx.EventManager().Events()), "no dns_update event expected on failure")
}

func TestMsgUpdateNoFeeWhenZero(t *testing.T) {
	f := initFixture(t)
	bank := newMockBankKeeper()
	f.keeper.SetBankKeeper(bank)

	setUpdateFee(t, f, 0)

	ownerAddr := sdk.AccAddress([]byte("owner________________"))
	owner, err := f.addressCodec.BytesToString(ownerAddr)
	require.NoError(t, err)

	initialBalance := sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewInt(123)))
	bank.setAccount(ownerAddr, initialBalance)
	bank.setModule(authtypes.FeeCollectorName, sdk.NewCoins())

	name := "example.lumen"
	err = f.keeper.Domain.Set(f.ctx, name, types.Domain{
		Index: name,
		Name:  name,
		Owner: owner,
		Records: []*types.Record{
			{Key: "cid", Value: "old", Ttl: 0},
		},
	})
	require.NoError(t, err)

	msg := &types.MsgUpdate{
		Creator: owner,
		Domain:  "example",
		Ext:     "lumen",
		Records: []*types.Record{
			{Key: "cid", Value: "new", Ttl: 0},
		},
	}

	_, err = keeper.NewMsgServerImpl(f.keeper).Update(f.ctx, msg)
	require.NoError(t, err)

	require.Equal(t, initialBalance, bank.getAccount(ownerAddr))
	require.Equal(t, sdk.NewCoins(), bank.modules[authtypes.FeeCollectorName])
	requireEventAttrs(t, f, "example.lumen", "0")
}

func TestMsgUpdateFailsOnInvalidCreatorAddress(t *testing.T) {
	f := initFixture(t)
	bank := newMockBankKeeper()
	f.keeper.SetBankKeeper(bank)

	setUpdateFee(t, f, 100)

	validOwnerAddr := sdk.AccAddress([]byte("valid_owner___________"))
	validOwner, err := f.addressCodec.BytesToString(validOwnerAddr)
	require.NoError(t, err)

	name := "example.lumen"
	err = f.keeper.Domain.Set(f.ctx, name, types.Domain{
		Index: name,
		Name:  name,
		Owner: validOwner,
		Records: []*types.Record{
			{Key: "cid", Value: "old", Ttl: 0},
		},
	})
	require.NoError(t, err)

	sdkCtx := sdk.UnwrapSDKContext(f.ctx)
	prevEvents := len(sdkCtx.EventManager().Events())

	msg := &types.MsgUpdate{
		Creator: "bad-address",
		Domain:  "example",
		Ext:     "lumen",
		Records: []*types.Record{{Key: "cid", Value: "new", Ttl: 0}},
	}

	_, err = keeper.NewMsgServerImpl(f.keeper).Update(f.ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid creator address")

	dom, err := f.keeper.Domain.Get(f.ctx, name)
	require.NoError(t, err)
	require.Equal(t, "old", dom.Records[0].Value)

	require.Equal(t, prevEvents, len(sdkCtx.EventManager().Events()))
	require.Nil(t, bank.modules[authtypes.FeeCollectorName])
}

func setUpdateFee(t *testing.T, f *fixture, fee uint64) {
	t.Helper()
	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.UpdatePowDifficulty = 0
	params.UpdateRateLimitSeconds = 0
	params.UpdateFeeUlmn = fee
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
}

func requireEventAttrs(t *testing.T, f *fixture, expectedName, expectedFee string) {
	t.Helper()
	sdkCtx := sdk.UnwrapSDKContext(f.ctx)
	events := sdkCtx.EventManager().Events()
	for _, ev := range events {
		if ev.Type != "dns_update" {
			continue
		}
		foundName := false
		foundFee := false
		for _, attr := range ev.Attributes {
			switch string(attr.Key) {
			case "name":
				require.Equal(t, expectedName, string(attr.Value))
				foundName = true
			case "fee_ulmn":
				require.Equal(t, expectedFee, string(attr.Value))
				foundFee = true
			}
		}
		if foundName && foundFee {
			return
		}
	}
	t.Fatalf("dns_update event with name=%s fee=%s not found", expectedName, expectedFee)
}
