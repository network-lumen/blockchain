package gateways

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
)

func (AppModule) GenerateGenesisState(simState *module.SimulationState) {}

func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	return nil
}

func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	return nil
}
