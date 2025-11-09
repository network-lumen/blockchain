package dns

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"lumen/x/dns/types"
)

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod:      "Resolve",
					Use:            "resolve [domain] [ext] [records] [expire-at] [status]",
					Short:          "Query resolve",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "records"}, {ProtoField: "expire_at"}, {ProtoField: "status"}},
				},

				{
					RpcMethod:      "DomainsByOwner",
					Use:            "domains-by-owner [owner]",
					Short:          "Query domains_by_owner",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}},
				},

				{
					RpcMethod:      "AuctionStatus",
					Use:            "auction-status [domain] [ext] [end] [highest-bid] [bidder]",
					Short:          "Query auction_status",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "end"}, {ProtoField: "highest_bid"}, {ProtoField: "bidder"}},
				},

				{
					RpcMethod:      "BaseFeeDns",
					Use:            "base-fee-dns [t] [alpha] [floor] [ceiling]",
					Short:          "Query base_fee_dns",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "t"}, {ProtoField: "alpha"}, {ProtoField: "floor"}, {ProtoField: "ceiling"}},
				},

				{
					RpcMethod: "ListDomain",
					Use:       "list-domain",
					Short:     "List all domain",
				},
				{
					RpcMethod:      "GetDomain",
					Use:            "get-domain [id]",
					Short:          "Gets a domain",
					Alias:          []string{"show-domain"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}},
				},
				{
					RpcMethod: "ListAuction",
					Use:       "list-auction",
					Short:     "List all auction",
				},
				{
					RpcMethod:      "GetAuction",
					Use:            "get-auction [id]",
					Short:          "Gets a auction",
					Alias:          []string{"show-auction"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true, // only required if you want to use the custom command
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod:      "Register",
					Use:            "register [domain] [ext] [records] [duration-days]",
					Short:          "Send a register tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "records"}, {ProtoField: "duration_days"}},
				},
				{
					RpcMethod:      "Update",
					Use:            "update [domain] [ext] [records]",
					Short:          "Send a update tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "records"}},
				},
				{
					RpcMethod:      "Renew",
					Use:            "renew [domain] [ext] [duration-days]",
					Short:          "Send a renew tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "duration_days"}},
				},
				{
					RpcMethod:      "Transfer",
					Use:            "transfer [domain] [ext] [new-owner]",
					Short:          "Send a transfer tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "new_owner"}},
				},
				{
					RpcMethod:      "Bid",
					Use:            "bid [domain] [ext] [amount]",
					Short:          "Send a bid tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "domain"}, {ProtoField: "ext"}, {ProtoField: "amount"}},
				},
				{
					RpcMethod:      "CreateDomain",
					Use:            "create-domain [index] [name] [owner] [records] [expire-at]",
					Short:          "Create a new domain",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}, {ProtoField: "name"}, {ProtoField: "owner"}, {ProtoField: "records"}, {ProtoField: "expire_at"}},
				},
				{
					RpcMethod:      "UpdateDomain",
					Use:            "update-domain [index] [name] [owner] [records] [expire-at]",
					Short:          "Update domain",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}, {ProtoField: "name"}, {ProtoField: "owner"}, {ProtoField: "records"}, {ProtoField: "expire_at"}},
				},
				{
					RpcMethod:      "DeleteDomain",
					Use:            "delete-domain [index]",
					Short:          "Delete domain",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}},
				},
				{
					RpcMethod:      "CreateAuction",
					Use:            "create-auction [index] [name] [start] [end] [highest-bid] [bidder]",
					Short:          "Create a new auction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}, {ProtoField: "name"}, {ProtoField: "start"}, {ProtoField: "end"}, {ProtoField: "highest_bid"}, {ProtoField: "bidder"}},
				},
				{
					RpcMethod:      "UpdateAuction",
					Use:            "update-auction [index] [name] [start] [end] [highest-bid] [bidder]",
					Short:          "Update auction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}, {ProtoField: "name"}, {ProtoField: "start"}, {ProtoField: "end"}, {ProtoField: "highest_bid"}, {ProtoField: "bidder"}},
				},
				{
					RpcMethod:      "DeleteAuction",
					Use:            "delete-auction [index]",
					Short:          "Delete auction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "index"}},
				},
				{
					RpcMethod: "Settle",
					Use:       "settle [domain] [ext]",
					Short:     "Finalize a finished auction and transfer ownership to the highest bidder",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "domain"}, {ProtoField: "ext"},
					},
				},
			},
		},
	}
}
