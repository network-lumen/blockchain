package app

import (
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type mockTx struct {
	msgs []sdk.Msg
}

func (m mockTx) GetMsgs() []sdk.Msg   { return m.msgs }
func (m mockTx) ValidateBasic() error { return nil }
func (m mockTx) GetMsgsV2() ([]proto.Message, error) {
	res := make([]proto.Message, len(m.msgs))
	for i, msg := range m.msgs {
		pm, ok := msg.(proto.Message)
		if !ok {
			return nil, fmt.Errorf("message %T does not implement proto.Message", msg)
		}
		res[i] = pm
	}
	return res, nil
}

func TestComputeSendTaxes_MsgSend(t *testing.T) {
	codec := address.NewBech32Codec(AccountAddressPrefix)
	senderAddr := sdk.AccAddress([]byte("sender____________"))
	recipientAddr := sdk.AccAddress([]byte("recipient_________"))
	sender, err := codec.BytesToString(senderAddr)
	require.NoError(t, err)
	recipient, err := codec.BytesToString(recipientAddr)
	require.NoError(t, err)

	tx := mockTx{msgs: []sdk.Msg{&banktypes.MsgSend{
		FromAddress: sender,
		ToAddress:   recipient,
		Amount:      sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1_000_000))),
	}}}

	rate := sdkmath.LegacyMustNewDecFromStr("0.01")
	taxes, total, err := computeSendTaxes(tx, rate, codec)
	require.NoError(t, err)

	record, ok := taxes[string(recipientAddr)]
	require.True(t, ok)
	require.Equal(t, recipientAddr, record.addr)
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(10_000))), record.coins)
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(10_000))), total)
}

func TestComputeSendTaxes_MsgMultiSend(t *testing.T) {
	codec := address.NewBech32Codec(AccountAddressPrefix)
	from := sdk.AccAddress([]byte("from_address_______"))
	to1 := sdk.AccAddress([]byte("to_one____________"))
	to2 := sdk.AccAddress([]byte("to_two____________"))

	fromStr, err := codec.BytesToString(from)
	require.NoError(t, err)
	to1Str, err := codec.BytesToString(to1)
	require.NoError(t, err)
	to2Str, err := codec.BytesToString(to2)
	require.NoError(t, err)

	tx := mockTx{msgs: []sdk.Msg{&banktypes.MsgMultiSend{
		Inputs: []banktypes.Input{{
			Address: fromStr,
			Coins:   sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(3_000_000))),
		}},
		Outputs: []banktypes.Output{
			{Address: to1Str, Coins: sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1_000_000)))},
			{Address: to2Str, Coins: sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(2_000_000)))},
		},
	}}}

	rate := sdkmath.LegacyMustNewDecFromStr("0.01")
	taxes, total, err := computeSendTaxes(tx, rate, codec)
	require.NoError(t, err)

	rec1 := taxes[string(to1)]
	require.NotNil(t, rec1)
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(10_000))), rec1.coins)

	rec2 := taxes[string(to2)]
	require.NotNil(t, rec2)
	require.Equal(t, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(20_000))), rec2.coins)

	require.Equal(t, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(30_000))), total)
}

func TestComputeSendTaxes_TooSmallAmounts(t *testing.T) {
	codec := address.NewBech32Codec(AccountAddressPrefix)
	senderAddr := sdk.AccAddress([]byte("sender_small_______"))
	recipientAddr := sdk.AccAddress([]byte("rcpt_small________"))
	sender, err := codec.BytesToString(senderAddr)
	require.NoError(t, err)
	recipient, err := codec.BytesToString(recipientAddr)
	require.NoError(t, err)

	tx := mockTx{msgs: []sdk.Msg{&banktypes.MsgSend{
		FromAddress: sender,
		ToAddress:   recipient,
		Amount:      sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(50))),
	}}}

	rate := sdkmath.LegacyMustNewDecFromStr("0.01")
	_, _, err = computeSendTaxes(tx, rate, codec)
	require.ErrorIs(t, err, ErrSendAmountTooSmall)
}
