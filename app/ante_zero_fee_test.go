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

func TestZeroFeeDecoratorRejectsNonZeroFee(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, log.NewNopLogger())
	decorator := NewZeroFeeDecorator()
	fee := sdk.NewCoins(sdk.NewInt64Coin("ulmn", 5))
	_, err := decorator.AnteHandle(ctx, mockFeeTx{fee: fee}, false, nextAnte)
	require.Error(t, err)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "gasless tx must have zero fee")
}

func TestZeroFeeDecoratorAllowsZeroFee(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, log.NewNopLogger())
	decorator := NewZeroFeeDecorator()
	_, err := decorator.AnteHandle(ctx, mockFeeTx{fee: sdk.NewCoins()}, false, nextAnte)
	require.NoError(t, err)
}
