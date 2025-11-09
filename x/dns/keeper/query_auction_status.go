package keeper

import (
	"context"

	"lumen/x/dns/types"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) AuctionStatus(ctx context.Context, req *types.QueryAuctionStatusRequest) (*types.QueryAuctionStatusResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	domain := types.NormalizeDomain(req.Domain)
	ext := types.NormalizeExt(req.Ext)
	if err := types.ValidateDomainParts(domain, ext); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	name := q.k.fqdn(domain, ext)

	if auc, err := q.k.Auction.Get(ctx, name); err == nil {
		return &types.QueryAuctionStatusResponse{Start: auc.Start}, nil
	}

	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "params not found")
	}
	dom, err := q.k.Domain.Get(ctx, name)
	if err != nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	start := dom.ExpireAt + params.GraceDays*24*3600
	return &types.QueryAuctionStatusResponse{Start: start}, nil
}
