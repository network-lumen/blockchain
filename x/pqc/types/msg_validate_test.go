package types

import (
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestMsgLinkAccountPQCSchemeLimit(t *testing.T) {
	msg := MsgLinkAccountPQC{
		Creator:  sdk.AccAddress(make([]byte, 20)).String(),
		Scheme:   "dilithium3",
		PubKey:   make([]byte, 64),
		PowNonce: []byte{0x01},
	}
	require.NoError(t, msg.ValidateBasic())

	msg.Scheme = strings.Repeat("x", PQCSchemeMaxLen+1)
	err := msg.ValidateBasic()
	require.Error(t, err)
	require.Contains(t, err.Error(), "scheme too long")
}

func TestMsgUpdateParamsValidateBasic(t *testing.T) {
	msg := MsgUpdateParams{
		Authority: sdk.AccAddress(make([]byte, 20)).String(),
		Params:    DefaultParams(),
	}
	require.NoError(t, msg.ValidateBasic())

	msg.Authority = "invalid"
	require.Error(t, msg.ValidateBasic())
}

func TestIBCRelayerAuthorityMessagesValidateBasic(t *testing.T) {
	authority := sdk.AccAddress(make([]byte, 20)).String()
	relayer := sdk.AccAddress(make([]byte, 20)).String()

	add := MsgAddIBCRelayer{Authority: authority, Relayer: relayer}
	require.NoError(t, add.ValidateBasic())

	remove := MsgRemoveIBCRelayer{Authority: authority, Relayer: relayer}
	require.NoError(t, remove.ValidateBasic())

	add.Relayer = "invalid"
	require.Error(t, add.ValidateBasic())

	remove.Authority = "invalid"
	require.Error(t, remove.ValidateBasic())
}
