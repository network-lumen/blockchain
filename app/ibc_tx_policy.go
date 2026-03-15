package app

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	clienttypesv2 "github.com/cosmos/ibc-go/v10/modules/core/02-client/v2/types"
	connectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	channeltypesv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"
)

type txFeePolicy int

const (
	txFeePolicyGasless txFeePolicy = iota
	txFeePolicyIBC
)

func classifyTxFeePolicy(msgs []sdk.Msg) (txFeePolicy, error) {
	var hasGasless bool
	var hasFeeBearingIBC bool

	for _, msg := range msgs {
		if isIBCFeeBearingMsg(msg) {
			hasFeeBearingIBC = true
			continue
		}
		hasGasless = true
	}

	if hasGasless && hasFeeBearingIBC {
		return txFeePolicyGasless, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "cannot mix fee-bearing IBC messages with gasless messages")
	}
	if hasFeeBearingIBC {
		return txFeePolicyIBC, nil
	}
	return txFeePolicyGasless, nil
}

func requiresIBCFee(msgs []sdk.Msg) (bool, error) {
	policy, err := classifyTxFeePolicy(msgs)
	if err != nil {
		return false, err
	}
	return policy == txFeePolicyIBC, nil
}

func allIBCRelayerCoreMsgs(msgs []sdk.Msg) bool {
	if len(msgs) == 0 {
		return false
	}
	for _, msg := range msgs {
		if !isIBCRelayerCoreMsg(msg) {
			return false
		}
	}
	return true
}

func isIBCFeeBearingMsg(msg sdk.Msg) bool {
	return isIBCTransferMsg(msg) || isIBCRelayerCoreMsg(msg)
}

func isIBCTransferMsg(msg sdk.Msg) bool {
	_, ok := msg.(*ibctransfertypes.MsgTransfer)
	return ok
}

func isIBCRelayerCoreMsg(msg sdk.Msg) bool {
	switch msg.(type) {
	case *clienttypes.MsgCreateClient:
		return true
	case *clienttypes.MsgUpdateClient:
		return true
	case *clienttypes.MsgUpgradeClient:
		return true
	case *clienttypes.MsgSubmitMisbehaviour:
		return true
	case *clienttypes.MsgRecoverClient:
		return true
	case *clienttypesv2.MsgRegisterCounterparty:
		return true
	case *clienttypesv2.MsgUpdateClientConfig:
		return true
	case *connectiontypes.MsgConnectionOpenInit:
		return true
	case *connectiontypes.MsgConnectionOpenTry:
		return true
	case *connectiontypes.MsgConnectionOpenAck:
		return true
	case *connectiontypes.MsgConnectionOpenConfirm:
		return true
	case *channeltypes.MsgChannelOpenInit:
		return true
	case *channeltypes.MsgChannelOpenTry:
		return true
	case *channeltypes.MsgChannelOpenAck:
		return true
	case *channeltypes.MsgChannelOpenConfirm:
		return true
	case *channeltypes.MsgChannelCloseInit:
		return true
	case *channeltypes.MsgChannelCloseConfirm:
		return true
	case *channeltypes.MsgRecvPacket:
		return true
	case *channeltypes.MsgTimeout:
		return true
	case *channeltypes.MsgTimeoutOnClose:
		return true
	case *channeltypes.MsgAcknowledgement:
		return true
	case *channeltypesv2.MsgRecvPacket:
		return true
	case *channeltypesv2.MsgTimeout:
		return true
	case *channeltypesv2.MsgAcknowledgement:
		return true
	default:
		return false
	}
}
