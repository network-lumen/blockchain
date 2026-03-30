package app

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"
)

type mockFeeTx struct {
	msgs []sdk.Msg
	fee  sdk.Coins
}

func (m mockFeeTx) GetMsgs() []sdk.Msg                                 { return m.msgs }
func (mockFeeTx) GetMsgsV2() ([]protov2.Message, error)                { return nil, nil }
func (mockFeeTx) ValidateBasic() error                                 { return nil }
func (mockFeeTx) GetMemo() string                                      { return "" }
func (mockFeeTx) GetSignaturesV2() ([]signingtypes.SignatureV2, error) { return nil, nil }
func (mockFeeTx) GetPubKeys() ([]cryptotypes.PubKey, error)            { return nil, nil }
func (mockFeeTx) GetGas() uint64                                       { return 0 }
func (m mockFeeTx) GetFee() sdk.Coins {
	if m.fee == nil {
		return sdk.NewCoins()
	}
	return m.fee
}
func (mockFeeTx) FeePayer() []byte               { return nil }
func (mockFeeTx) FeeGranter() []byte             { return nil }
func (mockFeeTx) GetTimeoutHeight() uint64       { return 0 }
func (mockFeeTx) GetTimeoutTimeStamp() time.Time { return time.Time{} }
func (mockFeeTx) GetUnordered() bool             { return false }
func (mockFeeTx) GetSigners() ([][]byte, error)  { return nil, nil }

func TestSelectiveFeeDecoratorRejectsNonZeroFeeForGaslessTx(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, log.NewNopLogger())
	decorator := NewSelectiveFeeDecorator()
	fee := sdk.NewCoins(sdk.NewInt64Coin("ulmn", 5))
	_, err := decorator.AnteHandle(ctx, mockFeeTx{fee: fee}, false, nextAnte)
	require.Error(t, err)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "gasless tx must have zero fee")
}

func TestSelectiveFeeDecoratorAllowsZeroFeeForGaslessTx(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, log.NewNopLogger())
	decorator := NewSelectiveFeeDecorator()
	_, err := decorator.AnteHandle(ctx, mockFeeTx{fee: sdk.NewCoins()}, false, nextAnte)
	require.NoError(t, err)
}

func TestSelectiveFeeDecoratorRequiresPositiveFeeForIBCTransfer(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, log.NewNopLogger())
	decorator := NewSelectiveFeeDecorator()
	msg := ibctransfertypes.NewMsgTransfer(
		"transfer",
		"channel-0",
		sdk.NewInt64Coin("ulmn", 1),
		sdk.AccAddress(make([]byte, 20)).String(),
		"cosmos1deadbeefdeadbeefdeadbeefdeadbeefm4k3up",
		clienttypes.ZeroHeight(),
		1,
		"",
	)

	_, err := decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins()}, false, nextAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "ibc tx must pay a positive ulmn fee")

	_, err = decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins(sdk.NewInt64Coin("uatom", 1))}, false, nextAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "ibc tx fee denom must be")

	_, err = decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins(sdk.NewInt64Coin("ulmn", 1))}, false, nextAnte)
	require.NoError(t, err)
}

func TestSelectiveFeeDecoratorRequiresExactFeeForEditValidator(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, log.NewNopLogger())
	decorator := NewSelectiveFeeDecorator()
	msg := &stakingtypes.MsgEditValidator{}

	_, err := decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins()}, false, nextAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "edit-validator tx must pay exactly 1000000ulmn")

	_, err = decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins(sdk.NewInt64Coin("uatom", editValidatorFixedFeeUlmn))}, false, nextAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "edit-validator tx must pay exactly 1000000ulmn")

	_, err = decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins(sdk.NewInt64Coin("ulmn", editValidatorFixedFeeUlmn-1))}, false, nextAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "edit-validator tx must pay exactly 1000000ulmn")

	_, err = decorator.AnteHandle(ctx, mockFeeTx{msgs: []sdk.Msg{msg}, fee: sdk.NewCoins(sdk.NewInt64Coin("ulmn", editValidatorFixedFeeUlmn))}, false, nextAnte)
	require.NoError(t, err)
}
