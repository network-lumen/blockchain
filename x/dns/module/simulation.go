package dns

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	dnssimulation "lumen/x/dns/simulation"
	"lumen/x/dns/types"
)

func randomAccAddress() string {
	pk := ed25519.GenPrivKey().PubKey()
	return sdk.AccAddress(pk.Address()).String()
}

func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	accs := make([]string, len(simState.Accounts))
	for i, acc := range simState.Accounts {
		accs[i] = acc.Address.String()
	}
	dnsGenesis := types.GenesisState{
		Params: types.DefaultParams(),
		DomainMap: []types.Domain{{Creator: randomAccAddress(),
			Index: "0",
		}, {Creator: randomAccAddress(),
			Index: "1",
		}}, AuctionMap: []types.Auction{{Creator: randomAccAddress(),
			Index: "0",
		}, {Creator: randomAccAddress(),
			Index: "1",
		}}}
	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(&dnsGenesis)
}

func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgRegister          = "op_weight_msg_dns"
		defaultWeightMsgRegister int = 100
	)

	var weightMsgRegister int
	simState.AppParams.GetOrGenerate(opWeightMsgRegister, &weightMsgRegister, nil,
		func(_ *rand.Rand) {
			weightMsgRegister = defaultWeightMsgRegister
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgRegister,
		dnssimulation.SimulateMsgRegister(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgUpdate          = "op_weight_msg_dns"
		defaultWeightMsgUpdate int = 100
	)

	var weightMsgUpdate int
	simState.AppParams.GetOrGenerate(opWeightMsgUpdate, &weightMsgUpdate, nil,
		func(_ *rand.Rand) {
			weightMsgUpdate = defaultWeightMsgUpdate
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgUpdate,
		dnssimulation.SimulateMsgUpdate(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgRenew          = "op_weight_msg_dns"
		defaultWeightMsgRenew int = 100
	)

	var weightMsgRenew int
	simState.AppParams.GetOrGenerate(opWeightMsgRenew, &weightMsgRenew, nil,
		func(_ *rand.Rand) {
			weightMsgRenew = defaultWeightMsgRenew
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgRenew,
		dnssimulation.SimulateMsgRenew(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgTransfer          = "op_weight_msg_dns"
		defaultWeightMsgTransfer int = 100
	)

	var weightMsgTransfer int
	simState.AppParams.GetOrGenerate(opWeightMsgTransfer, &weightMsgTransfer, nil,
		func(_ *rand.Rand) {
			weightMsgTransfer = defaultWeightMsgTransfer
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgTransfer,
		dnssimulation.SimulateMsgTransfer(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgBid          = "op_weight_msg_dns"
		defaultWeightMsgBid int = 100
	)

	var weightMsgBid int
	simState.AppParams.GetOrGenerate(opWeightMsgBid, &weightMsgBid, nil,
		func(_ *rand.Rand) {
			weightMsgBid = defaultWeightMsgBid
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgBid,
		dnssimulation.SimulateMsgBid(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	return operations
}

func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
