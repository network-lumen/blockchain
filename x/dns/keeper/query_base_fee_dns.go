package keeper

import (
	"context"

	"lumen/x/dns/types"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) BaseFeeDns(ctx context.Context, _ *types.QueryBaseFeeDnsRequest) (*types.QueryBaseFeeDnsResponse, error) {
	p, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "params not found")
	}
	return &types.QueryBaseFeeDnsResponse{
		BaseFeeDns: p.BaseFeeDns,
	}, nil
}
