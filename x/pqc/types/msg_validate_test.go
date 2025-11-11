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
