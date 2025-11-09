package app

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	gogoproto "github.com/gogo/protobuf/proto"

	dnstypes "lumen/x/dns/types"
	gatewaytypes "lumen/x/gateways/types"
)

func TestGaslessMsgTypesExist(t *testing.T) {
	interfaceRegistry := codectypes.NewInterfaceRegistry()

	std.RegisterInterfaces(interfaceRegistry)
	dnstypes.RegisterInterfaces(interfaceRegistry)
	gatewaytypes.RegisterInterfaces(interfaceRegistry)

	for _, url := range GaslessMsgTypes() {
		msg, err := instantiateGaslessMsg(interfaceRegistry, url)
		require.NoError(t, err, "resolve %s", url)
		require.Equal(t, url, sdk.MsgTypeURL(msg))
	}
}

func instantiateGaslessMsg(reg codectypes.InterfaceRegistry, url string) (sdk.Msg, error) {
	if resolver, ok := any(reg).(interface {
		Resolve(string) (gogoproto.Message, error)
	}); ok {
		if resolved, err := resolver.Resolve(url); err == nil {
			if msg, ok := resolved.(sdk.Msg); ok {
				return msg, nil
			}
			return nil, fmt.Errorf("resolved %s to %T, not sdk.Msg", url, resolved)
		}
	}
	msgAny := &codectypes.Any{TypeUrl: url}
	var msg sdk.Msg
	if err := reg.UnpackAny(msgAny, &msg); err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("unpacked %s but got nil", url)
	}
	return msg, nil
}
