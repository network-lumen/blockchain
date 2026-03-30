package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	clienttypesv2 "github.com/cosmos/ibc-go/v10/modules/core/02-client/v2/types"
	connectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	channeltypesv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"

	pqctypes "lumen/x/pqc/types"
)

func TestIBCRelayerCoreMessagesAreRecognized(t *testing.T) {
	for _, msg := range sampleIBCRelayerCoreMsgs() {
		require.Truef(t, isIBCRelayerCoreMsg(msg), "expected relayer core match for %T", msg)

		requiresFee, err := requiresIBCFee([]sdk.Msg{msg})
		require.NoError(t, err)
		require.Truef(t, requiresFee, "expected fee-bearing policy for %T", msg)
	}
}

func TestIBCTransferMessagesAreRecognized(t *testing.T) {
	msg := sampleIBCTransferMsg()

	require.True(t, isIBCTransferMsg(msg))
	require.True(t, isIBCFeeBearingMsg(msg))

	requiresFee, err := requiresIBCFee([]sdk.Msg{msg})
	require.NoError(t, err)
	require.True(t, requiresFee)
	require.False(t, allIBCRelayerCoreMsgs([]sdk.Msg{msg}))
}

func TestEditValidatorMessagesAreRecognized(t *testing.T) {
	msg := sampleEditValidatorMsg()

	require.True(t, isEditValidatorFeeBearingMsg(msg))

	policy, err := classifyTxFeePolicy([]sdk.Msg{msg})
	require.NoError(t, err)
	require.Equal(t, txFeePolicyEditValidator, policy)

	requiresFee, err := requiresIBCFee([]sdk.Msg{msg})
	require.NoError(t, err)
	require.False(t, requiresFee)
}

func TestShouldBypassPQCSigForIBCRelayerRequiresAllowlistedSigners(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	params := pqctypes.DefaultParams()
	params.IbcRelayerAllowlist = []string{signer.String()}

	for _, msg := range sampleIBCRelayerCoreMsgs() {
		require.Truef(
			t,
			shouldBypassPQCSigForIBCRelayer(params, []sdk.Msg{msg}, []sdk.AccAddress{signer}),
			"expected bypass for %T",
			msg,
		)
	}
}

func TestShouldBypassPQCSigForIBCRelayerRejectsUnknownSigner(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	_, _, otherSigner := testdata.KeyTestPubAddr()
	params := pqctypes.DefaultParams()
	params.IbcRelayerAllowlist = []string{signer.String()}

	for _, msg := range sampleIBCRelayerCoreMsgs() {
		require.Falsef(
			t,
			shouldBypassPQCSigForIBCRelayer(params, []sdk.Msg{msg}, []sdk.AccAddress{otherSigner}),
			"unexpected bypass for %T",
			msg,
		)
	}
}

func TestShouldBypassPQCSigForIBCRelayerRejectsMixedSignerSet(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	_, _, otherSigner := testdata.KeyTestPubAddr()
	params := pqctypes.DefaultParams()
	params.IbcRelayerAllowlist = []string{signer.String()}

	require.False(t, shouldBypassPQCSigForIBCRelayer(
		params,
		[]sdk.Msg{&channeltypes.MsgRecvPacket{}},
		[]sdk.AccAddress{signer, otherSigner},
	))
}

func TestShouldBypassPQCSigForIBCRelayerRejectsNonCoreIBCMessages(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	params := pqctypes.DefaultParams()
	params.IbcRelayerAllowlist = []string{signer.String()}

	require.False(t, shouldBypassPQCSigForIBCRelayer(params, []sdk.Msg{sampleIBCTransferMsg()}, []sdk.AccAddress{signer}))
}

func TestShouldBypassPQCSigForIBCRelayerRejectsMixedTransactions(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	_, _, recipient := testdata.KeyTestPubAddr()
	params := pqctypes.DefaultParams()
	params.IbcRelayerAllowlist = []string{signer.String()}

	mixed := []sdk.Msg{
		&channeltypes.MsgRecvPacket{},
		banktypes.NewMsgSend(signer, recipient, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 1))),
	}
	require.False(t, shouldBypassPQCSigForIBCRelayer(params, mixed, []sdk.AccAddress{signer}))
}

func TestClassifyTxFeePolicyRejectsMixedGaslessAndIBC(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	_, _, recipient := testdata.KeyTestPubAddr()

	_, err := classifyTxFeePolicy([]sdk.Msg{
		sampleIBCTransferMsg(),
		banktypes.NewMsgSend(signer, recipient, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 1))),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot mix fee-bearing IBC messages with gasless messages")
}

func TestClassifyTxFeePolicyAllowsIBCTransferAndCoreTogether(t *testing.T) {
	policy, err := classifyTxFeePolicy([]sdk.Msg{
		sampleIBCTransferMsg(),
		&channeltypes.MsgAcknowledgement{},
	})
	require.NoError(t, err)
	require.Equal(t, txFeePolicyIBC, policy)
}

func TestClassifyTxFeePolicyRejectsMixedGaslessAndEditValidator(t *testing.T) {
	_, _, signer := testdata.KeyTestPubAddr()
	_, _, recipient := testdata.KeyTestPubAddr()

	_, err := classifyTxFeePolicy([]sdk.Msg{
		sampleEditValidatorMsg(),
		banktypes.NewMsgSend(signer, recipient, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 1))),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot mix fee-bearing edit-validator messages with gasless messages")
}

func TestClassifyTxFeePolicyRejectsMixedIBCAndEditValidator(t *testing.T) {
	_, err := classifyTxFeePolicy([]sdk.Msg{
		sampleIBCTransferMsg(),
		sampleEditValidatorMsg(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot mix fee-bearing IBC messages with fee-bearing edit-validator messages")
}

func sampleIBCTransferMsg() sdk.Msg {
	return &ibctransfertypes.MsgTransfer{}
}

func sampleEditValidatorMsg() sdk.Msg {
	return &stakingtypes.MsgEditValidator{}
}

func sampleIBCRelayerCoreMsgs() []sdk.Msg {
	return []sdk.Msg{
		&clienttypes.MsgCreateClient{},
		&clienttypes.MsgUpdateClient{},
		&clienttypes.MsgUpgradeClient{},
		&clienttypes.MsgRecoverClient{},
		&clienttypesv2.MsgRegisterCounterparty{},
		&clienttypesv2.MsgUpdateClientConfig{},
		&connectiontypes.MsgConnectionOpenInit{},
		&connectiontypes.MsgConnectionOpenTry{},
		&connectiontypes.MsgConnectionOpenAck{},
		&connectiontypes.MsgConnectionOpenConfirm{},
		&channeltypes.MsgChannelOpenInit{},
		&channeltypes.MsgChannelOpenTry{},
		&channeltypes.MsgChannelOpenAck{},
		&channeltypes.MsgChannelOpenConfirm{},
		&channeltypes.MsgChannelCloseInit{},
		&channeltypes.MsgChannelCloseConfirm{},
		&channeltypes.MsgRecvPacket{},
		&channeltypes.MsgTimeout{},
		&channeltypes.MsgTimeoutOnClose{},
		&channeltypes.MsgAcknowledgement{},
		&channeltypesv2.MsgRecvPacket{},
		&channeltypesv2.MsgTimeout{},
		&channeltypesv2.MsgAcknowledgement{},
	}
}
