package pqc

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"lumen/x/pqc/types"
)

func (AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "AccountPQC",
					Use:            "account [addr]",
					Short:          "Query the PQC entry for an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "addr"}},
				},
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the current PQC parameters",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "LinkAccountPQC", Skip: true},
			},
		},
	}
}
