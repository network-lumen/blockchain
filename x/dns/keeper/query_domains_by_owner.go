package keeper

import (
	"context"

	"lumen/x/dns/types"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) DomainsByOwner(ctx context.Context, req *types.QueryDomainsByOwnerRequest) (*types.QueryDomainsByOwnerResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	var out []string
	err := q.k.Domain.Walk(ctx, nil, func(name string, dom types.Domain) (bool, error) {
		if dom.Owner == req.Owner {
			out = append(out, name)
		}
		return false, nil
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "walk: %v", err)
	}
	return &types.QueryDomainsByOwnerResponse{Domains: out}, nil
}
