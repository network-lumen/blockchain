package gateways

import (
	"lumen/x/gateways/types"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Show x/gateways module params"},
				{RpcMethod: "Gateways", Use: "gateways", Short: "List gateways"},
				{RpcMethod: "Gateway", Use: "gateway [id]", Short: "Get gateway by id", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Contracts", Use: "contracts", Short: "List contracts"},
				{RpcMethod: "Contract", Use: "contract [id]", Short: "Get contract by id", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Authority", Use: "authority", Short: "Show module authority"},
				{RpcMethod: "ModuleAccounts", Use: "module-accounts", Short: "Show module escrow/treasury accounts"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "RegisterGateway", Use: "register-gateway [payout]", Short: "Register a gateway"},
				{RpcMethod: "UpdateGateway", Use: "update-gateway [gateway_id]", Short: "Update a gateway"},
				{RpcMethod: "CreateContract", Use: "create-contract [gateway_id] [price_ulmn] [storage_gb] [network_gb] [months_total]", Short: "Create a gateway contract"},
				{RpcMethod: "ClaimPayment", Use: "claim-payment [contract_id]", Short: "Claim a monthly payout"},
				{RpcMethod: "CancelContract", Use: "cancel-contract [contract_id]", Short: "Cancel a contract"},
				{RpcMethod: "FinalizeContract", Use: "finalize-contract [contract_id]", Short: "Finalize a completed contract"},
				{RpcMethod: "UpdateParams", Use: "update-params", Short: "Update module params (gov authority only)"},
			},
		},
	}
}
