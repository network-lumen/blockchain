package cmd

import (
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"
)

func TestEnsureIBCClientInterfacesRegistered(t *testing.T) {
	reg := codectypes.NewInterfaceRegistry()

	ensureIBCClientInterfacesRegistered(reg)

	if err := reg.EnsureRegistered(&clienttypes.MsgCreateClient{}); err != nil {
		t.Fatalf("MsgCreateClient not registered: %v", err)
	}
	if err := reg.EnsureRegistered(&ibctransfertypes.MsgTransfer{}); err != nil {
		t.Fatalf("MsgTransfer not registered: %v", err)
	}
	if err := reg.EnsureRegistered(&ibctm.ClientState{}); err != nil {
		t.Fatalf("Tendermint ClientState not registered: %v", err)
	}
	if err := reg.EnsureRegistered(&ibctm.ConsensusState{}); err != nil {
		t.Fatalf("Tendermint ConsensusState not registered: %v", err)
	}
}
