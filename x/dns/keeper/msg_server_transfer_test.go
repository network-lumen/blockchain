package keeper_test

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"lumen/app/denom"
	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

type mockBankKeeper struct {
	accounts map[string]sdk.Coins
	modules  map[string]sdk.Coins
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{
		accounts: make(map[string]sdk.Coins),
		modules:  make(map[string]sdk.Coins),
	}
}

func (m *mockBankKeeper) keyAccount(addr sdk.AccAddress) string {
	return addr.String()
}

func (m *mockBankKeeper) getAccount(addr sdk.AccAddress) sdk.Coins {
	return m.accounts[m.keyAccount(addr)]
}

func (m *mockBankKeeper) setAccount(addr sdk.AccAddress, coins sdk.Coins) {
	m.accounts[m.keyAccount(addr)] = coins
}

func (m *mockBankKeeper) addAccount(addr sdk.AccAddress, coins sdk.Coins) {
	current := m.getAccount(addr)
	m.setAccount(addr, current.Add(coins...))
}

func (m *mockBankKeeper) setModule(module string, coins sdk.Coins) {
	m.modules[module] = coins
}

func (m *mockBankKeeper) addModule(module string, coins sdk.Coins) {
	current := m.modules[module]
	m.modules[module] = current.Add(coins...)
}

func (m *mockBankKeeper) SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins {
	return sdk.NewCoins()
}

func (m *mockBankKeeper) SendCoins(ctx context.Context, from sdk.AccAddress, to sdk.AccAddress, amt sdk.Coins) error {
	if err := m.SendCoinsFromAccountToModule(ctx, from, "__temp__", amt); err != nil {
		return err
	}
	return m.SendCoinsFromModuleToAccount(ctx, "__temp__", to, amt)
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, from sdk.AccAddress, module string, amt sdk.Coins) error {
	balance := m.getAccount(from)
	if !balance.IsAllGTE(amt) {
		return sdkerrors.ErrInsufficientFunds
	}
	m.setAccount(from, balance.Sub(amt...))
	m.addModule(module, amt)
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, module string, to sdk.AccAddress, amt sdk.Coins) error {
	balance := m.modules[module]
	if !balance.IsAllGTE(amt) {
		return sdkerrors.ErrInsufficientFunds
	}
	m.modules[module] = balance.Sub(amt...)
	m.addAccount(to, amt)
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, fromModule, toModule string, amt sdk.Coins) error {
	balance := m.modules[fromModule]
	if !balance.IsAllGTE(amt) {
		return sdkerrors.ErrInsufficientFunds
	}
	m.modules[fromModule] = balance.Sub(amt...)
	m.addModule(toModule, amt)
	return nil
}

func TestMsgTransferChargesFixedFee(t *testing.T) {
	f := initFixture(t)
	bank := newMockBankKeeper()
	f.keeper.SetBankKeeper(bank)

	creatorAddr := sdk.AccAddress([]byte("creator________________"))
	newOwnerAddr := sdk.AccAddress([]byte("new_owner____________"))

	creator, err := f.addressCodec.BytesToString(creatorAddr)
	require.NoError(t, err)
	newOwner, err := f.addressCodec.BytesToString(newOwnerAddr)
	require.NoError(t, err)

	fee := sdkmath.NewIntFromUint64(types.DefaultTransferFeeUlmn)
	bank.setAccount(creatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, fee.MulRaw(2))))
	bank.setModule(authtypes.FeeCollectorName, sdk.NewCoins())

	name := "example.lumen"
	err = f.keeper.Domain.Set(f.ctx, name, types.Domain{Index: name, Name: name, Owner: creator})
	require.NoError(t, err)

	msg := &types.MsgTransfer{
		Creator:  creator,
		Domain:   "example",
		Ext:      "lumen",
		NewOwner: newOwner,
	}

	_, err = keeper.NewMsgServerImpl(f.keeper).Transfer(f.ctx, msg)
	require.NoError(t, err)

	dom, err := f.keeper.Domain.Get(f.ctx, name)
	require.NoError(t, err)
	require.Equal(t, newOwner, dom.Owner)

	collected := bank.modules[authtypes.FeeCollectorName]
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, fee)), collected)
}
