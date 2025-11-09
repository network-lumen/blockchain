package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"cosmossdk.io/core/address"
	sdklog "cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	protov2 "google.golang.org/protobuf/proto"

	tokenomicstypes "lumen/x/tokenomics/types"
)

func TestMinSendDecoratorRejectsDust(t *testing.T) {
	ctx, decorator, accounts := setupMinSendDecorator(t)

	msg := banktypes.NewMsgSend(accounts.user1, accounts.user2, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, int64(tokenomicstypes.DefaultMinSendUlmn-1))))
	tx := minSendTx{msgs: []sdk.Msg{msg}}

	_, err := decorator.AnteHandle(ctx, tx, false, noOpAnteHandler)
	if err == nil {
		t.Fatalf("expected dust send to be rejected")
	}
	requireErrIs(t, err, sdkerrors.ErrInvalidRequest)
}

func TestMinSendDecoratorAllowsMinimum(t *testing.T) {
	ctx, decorator, accounts := setupMinSendDecorator(t)

	msg := banktypes.NewMsgSend(accounts.user1, accounts.user2, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, int64(tokenomicstypes.DefaultMinSendUlmn))))
	tx := minSendTx{msgs: []sdk.Msg{msg}}

	_, err := decorator.AnteHandle(ctx, tx, false, noOpAnteHandler)
	if err != nil {
		t.Fatalf("expected min send to pass: %v", err)
	}
}

func TestMinSendDecoratorMultiSendRejects(t *testing.T) {
	ctx, decorator, accounts := setupMinSendDecorator(t)

	input := banktypes.NewInput(accounts.user1, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, 2000)))
	outputSmall := banktypes.NewOutput(accounts.user2, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, 500)))
	outputOk := banktypes.NewOutput(accounts.user3, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, 1500)))
	msg := &banktypes.MsgMultiSend{
		Inputs:  []banktypes.Input{input},
		Outputs: []banktypes.Output{outputSmall, outputOk},
	}
	tx := minSendTx{msgs: []sdk.Msg{msg}}

	_, err := decorator.AnteHandle(ctx, tx, false, noOpAnteHandler)
	if err == nil {
		t.Fatalf("expected multisend dust to be rejected")
	}
	requireErrIs(t, err, sdkerrors.ErrInvalidRequest)
}

func TestMinSendDecoratorModuleExempt(t *testing.T) {
	ctx, decorator, accounts := setupMinSendDecorator(t)

	msg := banktypes.NewMsgSend(accounts.module, accounts.user2, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, 1)))
	tx := minSendTx{msgs: []sdk.Msg{msg}}

	_, err := decorator.AnteHandle(ctx, tx, false, noOpAnteHandler)
	if err != nil {
		t.Fatalf("expected module send to bypass filter: %v", err)
	}
}

// --- helpers ---

type minSendAccounts struct {
	user1  sdk.AccAddress
	user2  sdk.AccAddress
	user3  sdk.AccAddress
	module sdk.AccAddress
}

func setupMinSendDecorator(t *testing.T) (sdk.Context, MinSendDecorator, minSendAccounts) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey("min_send_test")
	memKey := storetypes.NewTransientStoreKey("min_send_test_transient")
	ctx := sdktestutil.DefaultContextWithDB(t, storeKey, memKey).Ctx.WithLogger(sdklog.NewNopLogger())

	codec := authcodec.NewBech32Codec(AccountAddressPrefix)
	accountKeeper := newMinSendAccountKeeperMock(codec)

	user1 := sdk.AccAddress(bytes.Repeat([]byte{0x01}, 20))
	user2 := sdk.AccAddress(bytes.Repeat([]byte{0x02}, 20))
	user3 := sdk.AccAddress(bytes.Repeat([]byte{0x03}, 20))
	module := authtypes.NewModuleAddress("fee_collector")

	accountKeeper.addAccount(authtypes.NewBaseAccountWithAddress(user1))
	accountKeeper.addAccount(authtypes.NewBaseAccountWithAddress(user2))
	accountKeeper.addAccount(authtypes.NewBaseAccountWithAddress(user3))
	accountKeeper.addAccount(authtypes.NewEmptyModuleAccount(authtypes.FeeCollectorName))

	params := tokenomicstypes.Params{
		TxTaxRate:                  tokenomicstypes.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  tokenomicstypes.DefaultInitialRewardPerBlockLumn,
		HalvingIntervalBlocks:      tokenomicstypes.DefaultHalvingIntervalBlocks,
		SupplyCapLumn:              tokenomicstypes.DefaultSupplyCapLumn,
		Decimals:                   tokenomicstypes.DefaultDecimals,
		MinSendUlmn:                tokenomicstypes.DefaultMinSendUlmn,
		Denom:                      tokenomicstypes.DefaultDenom,
		DistributionIntervalBlocks: tokenomicstypes.DefaultDistributionIntervalBlocks,
	}

	decorator := NewMinSendDecorator(accountKeeper, staticTokenomicsKeeper{params: params})

	return ctx, decorator, minSendAccounts{
		user1:  user1,
		user2:  user2,
		user3:  user3,
		module: module,
	}
}

var noOpAnteHandler = func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
	return ctx, nil
}

type minSendAccountKeeperMock struct {
	codec    address.Codec
	accounts map[string]sdk.AccountI
}

func newMinSendAccountKeeperMock(codec address.Codec) *minSendAccountKeeperMock {
	return &minSendAccountKeeperMock{
		codec:    codec,
		accounts: make(map[string]sdk.AccountI),
	}
}

func (m *minSendAccountKeeperMock) AddressCodec() address.Codec {
	return m.codec
}

func (m *minSendAccountKeeperMock) GetAccount(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	return m.accounts[addr.String()]
}

func (m *minSendAccountKeeperMock) addAccount(acc sdk.AccountI) {
	m.accounts[acc.GetAddress().String()] = acc
}

type staticTokenomicsKeeper struct {
	params tokenomicstypes.Params
}

func (s staticTokenomicsKeeper) GetParams(context.Context) tokenomicstypes.Params {
	return s.params
}

type minSendTx struct {
	msgs []sdk.Msg
}

var _ sdk.Tx = minSendTx{}
var _ authsigning.SigVerifiableTx = minSendTx{}

func (m minSendTx) GetMsgs() []sdk.Msg                                 { return m.msgs }
func (minSendTx) GetMsgsV2() ([]protov2.Message, error)                { return nil, nil }
func (minSendTx) ValidateBasic() error                                 { return nil }
func (minSendTx) GetMemo() string                                      { return "" }
func (minSendTx) GetSignaturesV2() ([]signingtypes.SignatureV2, error) { return nil, nil }
func (minSendTx) GetPubKeys() ([]cryptotypes.PubKey, error)            { return nil, nil }
func (minSendTx) GetGas() uint64                                       { return 0 }
func (minSendTx) GetFee() sdk.Coins                                    { return nil }
func (minSendTx) FeePayer() []byte                                     { return nil }
func (minSendTx) FeeGranter() []byte                                   { return nil }
func (minSendTx) GetTimeoutHeight() uint64                             { return 0 }
func (minSendTx) GetTimeoutTimeStamp() time.Time                       { return time.Time{} }
func (minSendTx) GetUnordered() bool                                   { return false }
func (minSendTx) GetSigners() ([][]byte, error)                        { return nil, nil }

// requireErrIs wraps testify helpers locally to avoid importing the full package twice.
func requireErrIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v, got %v", target, err)
	}
}
