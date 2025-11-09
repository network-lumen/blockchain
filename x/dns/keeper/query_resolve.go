package keeper

import (
	"context"

	"lumen/x/dns/types"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) Resolve(ctx context.Context, req *types.QueryResolveRequest) (*types.QueryResolveResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	domain := types.NormalizeDomain(req.Domain)
	ext := types.NormalizeExt(req.Ext)
	if err := types.ValidateDomainParts(domain, ext); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	name := q.k.fqdn(domain, ext)

	dom, err := q.k.Domain.Get(ctx, name)
	if err != nil {
		return nil, status.Error(codes.NotFound, "not found")
	}

	return &types.QueryResolveResponse{
		Owner: dom.Owner,
	}, nil
}
