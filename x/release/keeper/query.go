package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"lumen/x/release/types"
)

var _ types.QueryServer = queryServer{}

func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k} }

type queryServer struct{ k Keeper }

func (q queryServer) Release(ctx context.Context, req *types.QueryReleaseRequest) (*types.QueryReleaseResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	r, err := q.k.Release.Get(ctx, req.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	rr := r
	return &types.QueryReleaseResponse{Release: &rr}, nil
}

func (q queryServer) ReleaseByID(ctx context.Context, req *types.QueryReleaseByIDRequest) (*types.QueryReleaseResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	return q.Release(ctx, &types.QueryReleaseRequest{Id: req.Id})
}

func (q queryServer) Releases(ctx context.Context, req *types.QueryReleasesRequest) (*types.QueryReleasesResponse, error) {
	page := req.GetPage()
	limit := req.GetLimit()
	if limit == 0 {
		limit = 50
	}
	if page == 0 {
		page = 1
	}
	start := (page - 1) * limit

	total := uint64(0)
	items := make([]*types.Release, 0, limit)
	i := uint64(0)
	err := q.k.Release.Walk(ctx, nil, func(_ uint64, v types.Release) (bool, error) {
		if i >= start && uint64(len(items)) < limit {
			vv := v
			items = append(items, &vv)
		}
		i++
		total++
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &types.QueryReleasesResponse{Releases: items, Total: total}, nil
}

func (q queryServer) Latest(ctx context.Context, req *types.QueryLatestRequest) (*types.QueryReleaseResponse, error) {
	if req == nil || req.Channel == "" || req.Platform == "" || req.Kind == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	id, err := q.k.ByTriple.Get(ctx, tripleKey(req.Channel, req.Platform, req.Kind))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	r, err := q.k.Release.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	// Defense-in-depth: even if the index is polluted (e.g. pre-upgrade state),
	// Latest must never return a non-validated or yanked release.
	if r.Yanked || r.Status != types.Release_VALIDATED {
		return nil, status.Error(codes.NotFound, "not found")
	}
	rr := r
	return &types.QueryReleaseResponse{Release: &rr}, nil
}

func (q queryServer) LatestCanon(ctx context.Context, req *types.QueryLatestRequest) (*types.QueryReleaseResponse, error) {
	return q.Latest(ctx, req)
}

func (q queryServer) ByVersion(ctx context.Context, req *types.QueryByVersionRequest) (*types.QueryReleaseResponse, error) {
	if req == nil || req.Version == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	id, err := q.k.ByVersion.Get(ctx, req.Version)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("version %s not found", req.Version))
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	r, err := q.k.Release.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	rr := r
	return &types.QueryReleaseResponse{Release: &rr}, nil
}
