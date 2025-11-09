package keeper_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"lumen/app/denom"
	"lumen/x/gateways/keeper"
	gatewaysmodule "lumen/x/gateways/module"
	"lumen/x/gateways/types"
	tokenomicstypes "lumen/x/tokenomics/types"
)

type gatewayFixture struct {
	t            *testing.T
	ctx          sdk.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	bank         *mockBankKeeper
	tokenomics   *mockTokenomicsKeeper
}

func initGatewayFixture(t *testing.T) *gatewayFixture {
	t.Helper()

	encCfg := moduletestutil.MakeTestEncodingConfig(gatewaysmodule.AppModule{})
	addressCodec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	storeService := runtime.NewKVStoreService(storeKey)
	ctx := testutil.DefaultContextWithDB(t, storeKey, storetypes.NewTransientStoreKey("transient_test")).Ctx
	ctx = ctx.WithBlockTime(time.Unix(0, 0)).WithEventManager(sdk.NewEventManager())

	authority := authtypes.NewModuleAddress(types.GovModuleName)

	k := keeper.NewKeeper(storeService, encCfg.Codec, addressCodec, authority)

	bank := newMockBankKeeper()
	tokenomics := newMockTokenomicsKeeper()
	k.SetBankKeeper(bank)
	k.SetAccountKeeper(mockAccountKeeper{codec: addressCodec})
	k.SetTokenomicsKeeper(tokenomics)

	if err := k.Params.Set(ctx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set default params: %v", err)
	}

	return &gatewayFixture{
		t:            t,
		ctx:          ctx,
		keeper:       k,
		addressCodec: addressCodec,
		bank:         bank,
		tokenomics:   tokenomics,
	}
}

func randomAccAddress() string {
	pk := ed25519.GenPrivKey().PubKey()
	return sdk.AccAddress(pk.Address()).String()
}

func (f *gatewayFixture) resetEvents() {
	f.ctx = f.ctx.WithEventManager(sdk.NewEventManager())
}

func (f *gatewayFixture) withBlockTime(unix int64) {
	f.ctx = f.ctx.WithBlockTime(time.Unix(unix, 0))
}

func (f *gatewayFixture) registerFee() uint64 {
	return f.keeper.GetParams(f.ctx).RegisterGatewayFeeUlmn
}

func (f *gatewayFixture) mustAccAddress(addr string) sdk.AccAddress {
	bz, err := f.addressCodec.StringToBytes(addr)
	require.NoError(f.t, err, "invalid address %s", addr)
	return sdk.AccAddress(bz)
}

func TestCreateContractTracksEscrowAndGatewayCount(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)

	expectedRegisterFee := sdkmath.NewIntFromUint64(registerFee)
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, expectedRegisterFee)), f.bank.moduleBalance(authtypes.FeeCollectorName))

	gatewayID := resp.Id
	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	initialCoins := sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 5_000_000))
	f.bank.setAccountBalance(clientAddr, initialCoins)
	f.bank.setModuleBalance(types.ModuleAccountEscrow, sdk.NewCoins())
	f.bank.setModuleBalance(types.ModuleAccountTreasury, sdk.NewCoins())

	f.resetEvents()
	price := uint64(200_000)
	months := uint32(6)
	total := sdkmath.NewIntFromUint64(price).MulRaw(int64(months))
	tax := total.MulRaw(int64(tokenomicstypes.DefaultTxTaxRateBps)).QuoRaw(10_000)
	net := total.Sub(tax)
	netPerMonth := net.QuoRaw(int64(months))
	createResp, err := srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 20,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	require.Equal(t,
		sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, expectedRegisterFee.Add(tax))),
		f.bank.moduleBalance(authtypes.FeeCollectorName),
	)

	contract, err := f.keeper.Contracts.Get(f.ctx, createResp.ContractId)
	require.NoError(t, err)
	require.Equal(t, gatewayID, contract.GatewayId)
	require.Equal(t, uint32(0), contract.ClaimedMonths)
	require.Equal(t, types.ContractStatus_CONTRACT_STATUS_ACTIVE, contract.Status)
	require.Equal(t, net.Uint64(), f.bank.moduleBalance(types.ModuleAccountEscrow).AmountOf(denom.BaseDenom).Uint64())
	require.Equal(t, netPerMonth.Uint64(), contract.PriceUlmn)
	require.Equal(t, net.String(), contract.EscrowUlmn)

	gateway, err := f.keeper.Gateways.Get(f.ctx, gatewayID)
	require.NoError(t, err)
	require.Equal(t, uint32(1), gateway.ActiveClients)
	require.Equal(t, uint32(0), gateway.Cancellations)
}

func TestRegisterGatewayUsesParamFee(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	params.RegisterGatewayFeeUlmn = 5_500_000
	require.NoError(t, f.keeper.SetParams(f.ctx, params))

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(params.RegisterGatewayFeeUlmn))))

	_, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)

	moduleBal := f.bank.moduleBalance(authtypes.FeeCollectorName).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, params.RegisterGatewayFeeUlmn, moduleBal)
}

func TestCreateContractRejectsBelowMinimumPrice(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	f.bank.setAccountBalance(clientAddr, sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 1_000_000)))

	_, err = srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         resp.Id,
		PriceUlmn:         99_999,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 20,
		MonthsTotal:       1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price below minimum")
}

func TestCreateContractSplitsTaxAndEscrow(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)
	gatewayID := resp.Id

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	f.bank.setAccountBalance(clientAddr, sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 10_000_000)))

	price := uint64(5_000_000)
	months := uint32(1)
	f.resetEvents()
	_, err = srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 100,
		NetworkGbPerMonth: 100,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	total := sdkmath.NewIntFromUint64(price)
	expectedTax := total.MulRaw(int64(tokenomicstypes.DefaultTxTaxRateBps)).QuoRaw(10_000)
	expectedNet := total.Sub(expectedTax)

	feeCollectorBal := f.bank.moduleBalance(authtypes.FeeCollectorName).AmountOf(denom.BaseDenom)
	require.Equal(t, sdkmath.NewIntFromUint64(registerFee).Add(expectedTax), feeCollectorBal)

	escrowBal := f.bank.moduleBalance(types.ModuleAccountEscrow).AmountOf(denom.BaseDenom)
	require.Equal(t, expectedNet, escrowBal)
}

func TestCreateContractAppliesTaxOnTotalMonths(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)
	gatewayID := resp.Id

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	f.bank.setAccountBalance(clientAddr, sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 50_000_000)))

	price := uint64(3_333_333)
	months := uint32(3)
	f.resetEvents()
	_, err = srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 10,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	total := sdkmath.NewIntFromUint64(price).MulRaw(int64(months))
	expectedTax := total.MulRaw(int64(tokenomicstypes.DefaultTxTaxRateBps)).QuoRaw(10_000)
	expectedNet := total.Sub(expectedTax)

	feeCollectorBal := f.bank.moduleBalance(authtypes.FeeCollectorName).AmountOf(denom.BaseDenom)
	require.Equal(t, sdkmath.NewIntFromUint64(registerFee).Add(expectedTax), feeCollectorBal)

	escrowBal := f.bank.moduleBalance(types.ModuleAccountEscrow).AmountOf(denom.BaseDenom)
	require.Equal(t, expectedNet, escrowBal)
}

func TestCreateContractEmitsPaymentEvent(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)
	gatewayID := resp.Id

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	f.bank.setAccountBalance(clientAddr, sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 5_000_000)))

	price := uint64(500_000)
	months := uint32(2)
	total := sdkmath.NewIntFromUint64(price).MulRaw(int64(months))
	expectedTax := total.MulRaw(int64(tokenomicstypes.DefaultTxTaxRateBps)).QuoRaw(10_000)
	expectedNet := total.Sub(expectedTax)

	f.resetEvents()
	_, err = srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 10,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	events := f.ctx.EventManager().Events()
	require.Len(t, events, 2)
	payment := events[0]
	require.Equal(t, "contract_payment", payment.Type)
	attrs := map[string]string{}
	for _, attr := range payment.Attributes {
		attrs[string(attr.Key)] = string(attr.Value)
	}
	require.Equal(t, fmt.Sprintf("%d", price), attrs["price_ulmn"])
	require.Equal(t, fmt.Sprintf("%d", months), attrs["months_total"])
	require.Equal(t, expectedTax.String(), attrs["tax_ulmn"])
	require.Equal(t, expectedNet.String(), attrs["net_ulmn"])
	require.Equal(t, client, attrs["payer"])
}

func TestClaimPaymentPaysGatewayAndTreasury(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)
	gatewayID := resp.Id
	expectedRegisterFee := sdkmath.NewIntFromUint64(registerFee)
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, expectedRegisterFee)), f.bank.moduleBalance(authtypes.FeeCollectorName))

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	initialCoins := sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 5_000_000))
	f.bank.setAccountBalance(clientAddr, initialCoins)
	f.bank.setModuleBalance(types.ModuleAccountEscrow, sdk.NewCoins())
	f.bank.setModuleBalance(types.ModuleAccountTreasury, sdk.NewCoins())

	price := uint64(200_000)
	months := uint32(6)
	f.resetEvents()
	createResp, err := srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 20,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	params := f.keeper.GetParams(f.ctx)
	f.withBlockTime(int64(params.MonthSeconds))
	f.resetEvents()
	claimResp, err := srv.ClaimPayment(f.ctx, &types.MsgClaimPayment{
		Operator:   operator,
		ContractId: createResp.ContractId,
	})
	require.NoError(t, err)
	require.Equal(t, "196020", claimResp.PaidUlmn)

	contract, err := f.keeper.Contracts.Get(f.ctx, createResp.ContractId)
	require.NoError(t, err)
	require.Equal(t, uint32(1), contract.ClaimedMonths)
	require.Equal(t, types.ContractStatus_CONTRACT_STATUS_ACTIVE, contract.Status)
	require.Equal(t, "990000", contract.EscrowUlmn)

	escrowBal := f.bank.moduleBalance(types.ModuleAccountEscrow).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(990_000), escrowBal)
	treasuryBal := f.bank.moduleBalance(types.ModuleAccountTreasury).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(1_980), treasuryBal)

	operatorBal := f.bank.accountBalance(f.mustAccAddress(operator)).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(196_020), operatorBal)
}

func TestCancelContractRefundsAndPenalizes(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)
	gatewayID := resp.Id

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	initialCoins := sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 5_000_000))
	f.bank.setAccountBalance(clientAddr, initialCoins)
	f.bank.setModuleBalance(types.ModuleAccountEscrow, sdk.NewCoins())
	f.bank.setModuleBalance(types.ModuleAccountTreasury, sdk.NewCoins())

	price := uint64(200_000)
	months := uint32(6)
	f.resetEvents()
	createResp, err := srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 20,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	f.resetEvents()
	cancelResp, err := srv.CancelContract(f.ctx, &types.MsgCancelContract{
		Client:     client,
		ContractId: createResp.ContractId,
	})
	require.NoError(t, err)
	require.Equal(t, "990000", cancelResp.RefundedUlmn)

	contract, err := f.keeper.Contracts.Get(f.ctx, createResp.ContractId)
	require.NoError(t, err)
	require.Equal(t, types.ContractStatus_CONTRACT_STATUS_CANCELED, contract.Status)
	require.Equal(t, "0", contract.EscrowUlmn)

	gateway, err := f.keeper.Gateways.Get(f.ctx, gatewayID)
	require.NoError(t, err)
	require.Equal(t, uint32(0), gateway.ActiveClients)
	require.Equal(t, uint32(1), gateway.Cancellations)

	clientBal := f.bank.accountBalance(clientAddr).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(4_790_000), clientBal)
	operatorBal := f.bank.accountBalance(f.mustAccAddress(operator)).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(196_020), operatorBal)
	treasuryBal := f.bank.moduleBalance(types.ModuleAccountTreasury).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(1_980), treasuryBal)
	escrowBal := f.bank.moduleBalance(types.ModuleAccountEscrow).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(0), escrowBal)
}

func TestFinalizeContractRewardsAfterDelay(t *testing.T) {
	f := initGatewayFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	operator := randomAccAddress()
	operatorAddr := f.mustAccAddress(operator)
	registerFee := f.registerFee()
	f.bank.setAccountBalance(operatorAddr, sdk.NewCoins(sdk.NewCoin(denom.BaseDenom, sdkmath.NewIntFromUint64(registerFee))))
	f.resetEvents()
	resp, err := srv.RegisterGateway(f.ctx, &types.MsgRegisterGateway{Operator: operator})
	require.NoError(t, err)
	gatewayID := resp.Id

	client := randomAccAddress()
	clientAddr := f.mustAccAddress(client)
	initialCoins := sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 5_000_000))
	f.bank.setAccountBalance(clientAddr, initialCoins)
	f.bank.setModuleBalance(types.ModuleAccountEscrow, sdk.NewCoins())
	f.bank.setModuleBalance(types.ModuleAccountTreasury, sdk.NewCoins())

	price := uint64(200_000)
	months := uint32(1)
	f.resetEvents()
	createResp, err := srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Client:            client,
		GatewayId:         gatewayID,
		PriceUlmn:         price,
		StorageGbPerMonth: 10,
		NetworkGbPerMonth: 20,
		MonthsTotal:       months,
	})
	require.NoError(t, err)

	params := f.keeper.GetParams(f.ctx)
	f.withBlockTime(int64(params.MonthSeconds))
	f.resetEvents()
	_, err = srv.ClaimPayment(f.ctx, &types.MsgClaimPayment{
		Operator:   operator,
		ContractId: createResp.ContractId,
	})
	require.NoError(t, err)

	contract, err := f.keeper.Contracts.Get(f.ctx, createResp.ContractId)
	require.NoError(t, err)
	contract.EscrowUlmn = "50"
	require.NoError(t, f.keeper.Contracts.Set(f.ctx, contract.Id, contract))
	f.bank.addModuleBalance(types.ModuleAccountEscrow, sdk.NewCoins(sdk.NewInt64Coin(denom.BaseDenom, 50)))

	finalizer := randomAccAddress()
	totalDelay := int64(params.MonthSeconds * (uint64(params.FinalizeDelayMonths) + uint64(months)))
	f.withBlockTime(totalDelay)
	f.resetEvents()
	finalResp, err := srv.FinalizeContract(f.ctx, &types.MsgFinalizeContract{
		Finalizer:  finalizer,
		ContractId: createResp.ContractId,
	})
	require.NoError(t, err)
	require.Equal(t, "2", finalResp.RewardUlmn)

	contract, err = f.keeper.Contracts.Get(f.ctx, createResp.ContractId)
	require.NoError(t, err)
	require.Equal(t, types.ContractStatus_CONTRACT_STATUS_FINALIZED, contract.Status)
	require.Equal(t, "0", contract.EscrowUlmn)

	gateway, err := f.keeper.Gateways.Get(f.ctx, gatewayID)
	require.NoError(t, err)
	require.Equal(t, uint32(0), gateway.ActiveClients)

	finalizerBal := f.bank.accountBalance(f.mustAccAddress(finalizer)).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(2), finalizerBal)
	clientBal := f.bank.accountBalance(clientAddr).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(4_800_048), clientBal)
	escrowBal := f.bank.moduleBalance(types.ModuleAccountEscrow).AmountOf(denom.BaseDenom).Uint64()
	require.Equal(t, uint64(0), escrowBal)
}

type mockAccountKeeper struct {
	codec address.Codec
}

func (m mockAccountKeeper) AddressCodec() address.Codec {
	return m.codec
}

type mockBankKeeper struct {
	balances map[string]sdk.Coins
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{balances: make(map[string]sdk.Coins)}
}

func (m *mockBankKeeper) keyAccount(addr sdk.AccAddress) string {
	return fmt.Sprintf("acc:%s", addr.String())
}

func (m *mockBankKeeper) keyModule(module string) string {
	return fmt.Sprintf("mod:%s", module)
}

func (m *mockBankKeeper) getBalance(key string) sdk.Coins {
	if bal, ok := m.balances[key]; ok {
		return bal
	}
	return sdk.NewCoins()
}

func (m *mockBankKeeper) setBalance(key string, coins sdk.Coins) {
	m.balances[key] = coins.Sort()
}

func (m *mockBankKeeper) addBalance(key string, coins sdk.Coins) {
	current := m.getBalance(key)
	m.balances[key] = current.Add(coins...).Sort()
}

func (m *mockBankKeeper) EnsureBalance(from string, coins sdk.Coins) error {
	if !m.getBalance(from).IsAllGTE(coins) {
		return fmt.Errorf("insufficient funds in %s", from)
	}
	return nil
}

func (m *mockBankKeeper) transfer(fromKey, toKey string, coins sdk.Coins) error {
	if !coins.IsValid() {
		return fmt.Errorf("invalid coins")
	}
	coins = coins.Sort()
	if err := m.EnsureBalance(fromKey, coins); err != nil {
		return err
	}
	fromBal := m.getBalance(fromKey)
	toBal := m.getBalance(toKey)
	m.balances[fromKey] = fromBal.Sub(coins...).Sort()
	m.balances[toKey] = toBal.Add(coins...).Sort()
	return nil
}

func (m *mockBankKeeper) setAccountBalance(addr sdk.AccAddress, coins sdk.Coins) {
	m.setBalance(m.keyAccount(addr), coins)
}

func (m *mockBankKeeper) accountBalance(addr sdk.AccAddress) sdk.Coins {
	return m.getBalance(m.keyAccount(addr))
}

func (m *mockBankKeeper) setModuleBalance(module string, coins sdk.Coins) {
	m.setBalance(m.keyModule(module), coins)
}

func (m *mockBankKeeper) moduleBalance(module string) sdk.Coins {
	return m.getBalance(m.keyModule(module))
}

func (m *mockBankKeeper) addModuleBalance(module string, coins sdk.Coins) {
	m.addBalance(m.keyModule(module), coins)
}

func (m *mockBankKeeper) SpendableCoins(_ context.Context, addr sdk.AccAddress) sdk.Coins {
	return m.accountBalance(addr)
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(ctx context.Context, addr sdk.AccAddress, module string, coins sdk.Coins) error {
	return m.transfer(m.keyAccount(addr), m.keyModule(module), coins)
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(ctx context.Context, module string, addr sdk.AccAddress, coins sdk.Coins) error {
	return m.transfer(m.keyModule(module), m.keyAccount(addr), coins)
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, coins sdk.Coins) error {
	return m.transfer(m.keyModule(senderModule), m.keyModule(recipientModule), coins)
}

type mockTokenomicsKeeper struct {
	params tokenomicstypes.Params
}

func newMockTokenomicsKeeper() *mockTokenomicsKeeper {
	return &mockTokenomicsKeeper{params: tokenomicstypes.DefaultParams()}
}

func (m *mockTokenomicsKeeper) GetParams(context.Context) tokenomicstypes.Params {
	return m.params
}
