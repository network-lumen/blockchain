package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"lumen/x/pqc/types"
)

type queryServer struct {
	keeper Keeper
}

func NewQueryServerImpl(k Keeper) types.QueryServer {
	return &queryServer{keeper: k}
}

func (q queryServer) AccountPQC(goCtx context.Context, req *types.QueryAccountPQCRequest) (*types.QueryAccountPQCResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	if len(req.Addr) == 0 {
		return nil, status.Error(codes.InvalidArgument, "addr is required")
	}

	addr, err := sdk.AccAddressFromBech32(req.Addr)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "addr: %v", err)
	}

	account, found, err := q.keeper.GetAccountPQC(goCtx, addr)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsmod.Wrapf(sdkerrors.ErrNotFound, "no pqc record for %s", req.Addr)
	}

	return &types.QueryAccountPQCResponse{Account: account}, nil
}

func (q queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params := q.keeper.GetParams(goCtx)
	return &types.QueryParamsResponse{Params: params}, nil
}
