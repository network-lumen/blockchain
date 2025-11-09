package keeper

import "lumen/x/dns/types"

type queryServer struct {
	k Keeper
}

var _ types.QueryServer = queryServer{}

func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{k: k}
}
