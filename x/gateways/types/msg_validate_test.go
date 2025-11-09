package types

import (
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestMsgRegisterGatewayMetadataBounds(t *testing.T) {
	op := sampleAddr(1)
	payout := sampleAddr(2)
	msg := MsgRegisterGateway{
		Operator: op,
		Payout:   payout,
		Metadata: strings.Repeat("a", GatewayMetadataMaxLen),
	}
	require.NoError(t, msg.ValidateBasic())

	msg.Metadata = strings.Repeat("a", GatewayMetadataMaxLen+1)
	err := msg.ValidateBasic()
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata too long")
}

func sampleAddr(seed byte) string {
	bz := make([]byte, 20)
	bz[19] = seed
	return sdk.AccAddress(bz).String()
}
