package release

import (
	"lumen/x/release/types"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Show release module params"},
				{RpcMethod: "Release", Use: "release [id]", Short: "Get a release by id", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Releases", Use: "releases", Short: "List releases (page/limit via flags)"},
				{RpcMethod: "Latest", Use: "latest [channel] [platform] [kind]", Short: "Get latest release for triple", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "channel"}, {ProtoField: "platform"}, {ProtoField: "kind"}}},
				{RpcMethod: "ByVersion", Use: "by-version [version]", Short: "Get by version", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "version"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "UpdateParams", Skip: true},
				{RpcMethod: "PublishRelease", Use: "publish", Short: "Publish a new release"},
				{RpcMethod: "YankRelease", Use: "yank [id]", Short: "Yank a release", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "MirrorRelease", Use: "mirror [id] [artifact-index] [new-urls]", Short: "Mirror release artifact", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}, {ProtoField: "artifact_index"}, {ProtoField: "new_urls"}}},
			},
		},
	}
}
