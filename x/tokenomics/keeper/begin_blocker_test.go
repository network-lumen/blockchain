package keeper_test

import (
	"context"
	"testing"

	address "cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"lumen/x/tokenomics/keeper"
	module "lumen/x/tokenomics/module"
	"lumen/x/tokenomics/types"
)

type bankMock struct {
	minted    map[string]sdk.Coins
	transfers []transfer
}

type transfer struct {
	from string
	to   string
	amt  sdk.Coins
}

func newBankMock() *bankMock {
	return &bankMock{
		minted:    make(map[string]sdk.Coins),
		transfers: []transfer{},
	}
}

func (m *bankMock) MintCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	current := m.minted[moduleName]
	m.minted[moduleName] = current.Add(amt...)
	return nil
}

func (m *bankMock) SendCoinsFromModuleToModule(_ context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	if !m.minted[senderModule].IsAllGTE(amt) {
		panic("insufficient balance in mock bank")
	}
	m.minted[senderModule] = m.minted[senderModule].Sub(amt...)
	m.minted[recipientModule] = m.minted[recipientModule].Add(amt...)
	m.transfers = append(m.transfers, transfer{from: senderModule, to: recipientModule, amt: amt})
	return nil
}

type distributionMock struct {
	calls              []sdk.ValAddress
	communityPoolMints []sdk.Coins
}

func newDistributionMock() *distributionMock {
	return &distributionMock{
		calls:              []sdk.ValAddress{},
		communityPoolMints: []sdk.Coins{},
	}
}

func (m *distributionMock) WithdrawValidatorCommission(_ context.Context, valAddr sdk.ValAddress) (sdk.Coins, error) {
	m.calls = append(m.calls, valAddr)
	return sdk.NewCoins(), nil
}

func (m *distributionMock) FundCommunityPool(_ context.Context, amount sdk.Coins, _ sdk.AccAddress) error {
	m.communityPoolMints = append(m.communityPoolMints, amount)
	return nil
}

type stakingMock struct {
	validators []stakingtypes.ValidatorI
	codec      address.Codec
}

func newStakingMock(codec address.Codec, validators []stakingtypes.ValidatorI) *stakingMock {
	return &stakingMock{validators: validators, codec: codec}
}

func (s *stakingMock) IterateValidators(_ context.Context, fn func(index int64, validator stakingtypes.ValidatorI) (stop bool)) {
	for i, v := range s.validators {
		if fn(int64(i), v) {
			break
		}
	}
}

func (s *stakingMock) ValidatorAddressCodec() address.Codec {
	return s.codec
}

type slashingMock struct {
	params slashingtypes.Params
}

func newSlashingMock() *slashingMock {
	return &slashingMock{
		params: slashingtypes.DefaultParams(),
	}
}

func (s *slashingMock) GetParams(_ context.Context) (slashingtypes.Params, error) {
	return s.params, nil
}

func (s *slashingMock) SetParams(_ context.Context, params slashingtypes.Params) error {
	s.params = params
	return nil
}

type fixture struct {
	sdkCtx sdk.Context
	keeper keeper.Keeper
	bank   *bankMock
	dist   *distributionMock
	stake  *stakingMock
	slash  *slashingMock
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount("lmn", "lmnpub")
	cfg.SetBech32PrefixForValidator("lmnvaloper", "lmnvaloperpub")
	cfg.SetBech32PrefixForConsensusNode("lmnvalcons", "lmnvalconspub")

	encCfg := moduletestutil.MakeTestEncodingConfig(module.AppModule{})
	addrCodec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	storeService := runtime.NewKVStoreService(storeKey)
	sdkCtx := testutil.DefaultContextWithDB(t, storeKey, storetypes.NewTransientStoreKey("transient_test")).Ctx

	authority := authtypes.NewModuleAddress(types.GovModuleName)

	k := keeper.NewKeeper(storeService, encCfg.Codec, addrCodec, authority)
	bank := newBankMock()
	k.SetBankKeeper(bank)

	valCodec := addresscodec.NewBech32Codec(cfg.GetBech32ValidatorAddrPrefix())
	pk := ed25519.GenPrivKey().PubKey()
	valAddr := sdk.ValAddress(pk.Address())
	validator, err := stakingtypes.NewValidator(valAddr.String(), pk, stakingtypes.Description{})
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}
	validator.Status = stakingtypes.Bonded

	dist := newDistributionMock()
	stake := newStakingMock(valCodec, []stakingtypes.ValidatorI{validator})
	k.SetDistributionKeeper(dist)
	k.SetStakingKeeper(stake)

	slash := newSlashingMock()
	k.SetSlashingKeeper(slash)

	if err := k.SetParams(sdkCtx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set params: %v", err)
	}
	if err := k.SetTotalMintedUlmn(sdkCtx, sdkmath.ZeroInt()); err != nil {
		t.Fatalf("failed to set total minted: %v", err)
	}

	return &fixture{
		sdkCtx: sdkCtx,
		keeper: k,
		bank:   bank,
		dist:   dist,
		stake:  stake,
		slash:  slash,
	}
}

func TestBeginBlocker_MintsAndRoutes(t *testing.T) {
	f := initFixture(t)

	params := types.Params{
		TxTaxRate:                  types.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  1,
		HalvingIntervalBlocks:      100,
		SupplyCapLumn:              10,
		Decimals:                   types.DefaultDecimals,
		Denom:                      types.DefaultDenom,
		DistributionIntervalBlocks: types.DefaultDistributionIntervalBlocks,
		MinSendUlmn:                types.DefaultMinSendUlmn,
	}
	if err := f.keeper.SetParams(f.sdkCtx, params); err != nil {
		t.Fatalf("set params: %v", err)
	}

	f.keeper.BeginBlocker(f.sdkCtx)

	minted := f.bank.minted[authtypes.FeeCollectorName]
	expected := sdk.NewCoin(types.DefaultDenom, sdkmath.NewIntWithDecimal(1, int(params.Decimals)))
	if !minted.AmountOf(types.DefaultDenom).Equal(expected.Amount) {
		t.Fatalf("expected minted %s, got %s", expected.Amount, minted.AmountOf(types.DefaultDenom))
	}
	total := f.keeper.GetTotalMintedUlmn(f.sdkCtx)
	if !total.Equal(expected.Amount) {
		t.Fatalf("total minted mismatch: got %s want %s", total, expected.Amount)
	}
}

func TestBeginBlocker_Halving(t *testing.T) {
	f := initFixture(t)

	params := types.Params{
		TxTaxRate:                  types.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  8,
		HalvingIntervalBlocks:      5,
		SupplyCapLumn:              100,
		Decimals:                   types.DefaultDecimals,
		Denom:                      types.DefaultDenom,
		DistributionIntervalBlocks: types.DefaultDistributionIntervalBlocks,
		MinSendUlmn:                types.DefaultMinSendUlmn,
	}
	if err := f.keeper.SetParams(f.sdkCtx, params); err != nil {
		t.Fatalf("set params: %v", err)
	}

	f.sdkCtx = f.sdkCtx.WithBlockHeight(10) // epoch = 2
	f.keeper.BeginBlocker(f.sdkCtx)

	minted := f.bank.minted[authtypes.FeeCollectorName].AmountOf(types.DefaultDenom)
	expected := sdkmath.NewIntWithDecimal(8, int(params.Decimals)).QuoRaw(4)
	if !minted.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, minted)
	}
}

func TestBeginBlocker_CapRespected(t *testing.T) {
	f := initFixture(t)

	params := types.Params{
		TxTaxRate:                  types.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  5,
		HalvingIntervalBlocks:      100,
		SupplyCapLumn:              10,
		Decimals:                   types.DefaultDecimals,
		Denom:                      types.DefaultDenom,
		DistributionIntervalBlocks: types.DefaultDistributionIntervalBlocks,
		MinSendUlmn:                types.DefaultMinSendUlmn,
	}
	if err := f.keeper.SetParams(f.sdkCtx, params); err != nil {
		t.Fatalf("set params: %v", err)
	}

	mintedSoFar := sdkmath.NewIntWithDecimal(9, int(params.Decimals))
	if err := f.keeper.SetTotalMintedUlmn(f.sdkCtx, mintedSoFar); err != nil {
		t.Fatalf("set total minted: %v", err)
	}

	f.keeper.BeginBlocker(f.sdkCtx)

	minted := f.bank.minted[authtypes.FeeCollectorName].AmountOf(types.DefaultDenom)
	if !minted.Equal(sdkmath.NewIntWithDecimal(1, int(params.Decimals))) {
		t.Fatalf("expected 1 remaining mint, got %s", minted)
	}

	f.bank.minted = make(map[string]sdk.Coins)
	f.keeper.BeginBlocker(f.sdkCtx.WithBlockHeight(f.sdkCtx.BlockHeight() + 1))
	if amount := f.bank.minted[authtypes.FeeCollectorName].AmountOf(types.DefaultDenom); !amount.IsZero() {
		t.Fatalf("expected no mint after cap, got %s", amount)
	}
}

func TestBeginBlocker_DistributionInterval(t *testing.T) {
	f := initFixture(t)

	params := types.Params{
		TxTaxRate:                  types.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  1,
		HalvingIntervalBlocks:      100,
		SupplyCapLumn:              10,
		Decimals:                   types.DefaultDecimals,
		Denom:                      types.DefaultDenom,
		DistributionIntervalBlocks: 5,
		MinSendUlmn:                types.DefaultMinSendUlmn,
	}
	if err := f.keeper.SetParams(f.sdkCtx, params); err != nil {
		t.Fatalf("set params: %v", err)
	}

	f.sdkCtx = f.sdkCtx.WithBlockHeight(5)
	f.keeper.BeginBlocker(f.sdkCtx)

	if len(f.dist.calls) != 1 {
		t.Fatalf("expected distribution withdraw to be triggered once, got %d", len(f.dist.calls))
	}
}

func TestBeginBlocker_MintsCommunityPoolOnDoubleSignSlash(t *testing.T) {
	f := initFixture(t)

	params := types.Params{
		TxTaxRate:                  types.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  0,
		HalvingIntervalBlocks:      0,
		SupplyCapLumn:              0,
		Decimals:                   types.DefaultDecimals,
		Denom:                      types.DefaultDenom,
		DistributionIntervalBlocks: 0,
		MinSendUlmn:                types.DefaultMinSendUlmn,
	}
	if err := f.keeper.SetParams(f.sdkCtx, params); err != nil {
		t.Fatalf("set params: %v", err)
	}

	burned := sdk.NewCoin(types.DefaultDenom, sdkmath.NewInt(1000))
	f.sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			slashingtypes.EventTypeSlash,
			sdk.NewAttribute(slashingtypes.AttributeKeyReason, slashingtypes.AttributeValueDoubleSign),
			sdk.NewAttribute(slashingtypes.AttributeKeyBurnedCoins, burned.String()),
		),
	)

	f.keeper.BeginBlocker(f.sdkCtx)

	if len(f.dist.communityPoolMints) != 1 {
		t.Fatalf("expected 1 community pool mint, got %d", len(f.dist.communityPoolMints))
	}
	got := f.dist.communityPoolMints[0].AmountOf(types.DefaultDenom)
	if !got.Equal(burned.Amount) {
		t.Fatalf("expected community pool mint %s, got %s", burned.Amount, got)
	}
}
