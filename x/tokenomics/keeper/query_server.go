package keeper

import (
	"context"

	"lumen/x/tokenomics/types"
)

type queryServer struct{ Keeper }

var _ types.QueryServer = queryServer{}

func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return queryServer{Keeper: keeper}
}

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params := q.GetParams(ctx)
	return &types.QueryParamsResponse{Params: params}, nil
}
