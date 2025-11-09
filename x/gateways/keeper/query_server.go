package keeper

import (
	"context"
	"strings"

	"lumen/x/gateways/types"

	errorsmod "cosmossdk.io/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

const (
	DefaultQueryLimit uint64 = 50
	MaxQueryLimit     uint64 = 200
)

func clampLimit(requested uint64) uint64 {
	limit := requested
	if limit == 0 {
		limit = DefaultQueryLimit
	}
	if limit > MaxQueryLimit {
		limit = MaxQueryLimit
	}
	return limit
}

type queryServer struct{ Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer {
	return &queryServer{Keeper: k}
}

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params := q.GetParams(ctx)
	return &types.QueryParamsResponse{Params: &params}, nil
}

func (q queryServer) Authority(ctx context.Context, _ *types.QueryAuthorityRequest) (*types.QueryAuthorityResponse, error) {
	addr, _ := q.addressCodec.BytesToString(q.GetAuthority())
	return &types.QueryAuthorityResponse{Address: addr}, nil
}

func (q queryServer) ModuleAccounts(ctx context.Context, _ *types.QueryModuleAccountsRequest) (*types.QueryModuleAccountsResponse, error) {
	treasury := authtypes.NewModuleAddress(types.ModuleAccountTreasury)
	escrow := authtypes.NewModuleAddress(types.ModuleAccountEscrow)

	ts, _ := q.ak.AddressCodec().BytesToString(treasury)
	es, _ := q.ak.AddressCodec().BytesToString(escrow)

	return &types.QueryModuleAccountsResponse{
		Escrow:   es,
		Treasury: ts,
	}, nil
}

func (q queryServer) Gateway(ctx context.Context, req *types.QueryGatewayRequest) (*types.QueryGatewayResponse, error) {
	gateway, err := q.Keeper.Gateways.Get(ctx, req.Id)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "gateway not found")
	}
	return &types.QueryGatewayResponse{Gateway: &gateway}, nil
}

func (q queryServer) Gateways(ctx context.Context, req *types.QueryGatewaysRequest) (*types.QueryGatewaysResponse, error) {
	limit := clampLimit(req.Limit)

	offset := req.Offset
	total := uint64(0)
	collected := make([]*types.Gateway, 0, limit)

	_ = q.Keeper.Gateways.Walk(ctx, nil, func(_ uint64, gateway types.Gateway) (bool, error) {
		total++
		if total <= offset {
			return false, nil
		}
		if uint64(len(collected)) >= limit {
			return true, nil
		}
		gw := gateway
		collected = append(collected, &gw)
		return false, nil
	})

	return &types.QueryGatewaysResponse{
		Gateways: collected,
		Total:    total,
	}, nil
}

func (q queryServer) Contract(ctx context.Context, req *types.QueryContractRequest) (*types.QueryContractResponse, error) {
	contract, err := q.Keeper.Contracts.Get(ctx, req.Id)
	if err != nil {
		return nil, errorsmod.Wrap(types.ErrNotFound, "contract not found")
	}
	return &types.QueryContractResponse{Contract: &contract}, nil
}

func (q queryServer) Contracts(ctx context.Context, req *types.QueryContractsRequest) (*types.QueryContractsResponse, error) {
	limit := clampLimit(req.Limit)
	offset := req.Offset
	status := strings.TrimSpace(strings.ToUpper(req.Status))
	client := strings.TrimSpace(req.Client)
	gatewayID := req.GatewayId

	total := uint64(0)
	collected := make([]*types.Contract, 0, limit)

	_ = q.Keeper.Contracts.Walk(ctx, nil, func(_ uint64, contract types.Contract) (bool, error) {
		if status != "" && contract.Status.String() != "CONTRACT_STATUS_"+status {
			return false, nil
		}
		if client != "" && contract.Client != client {
			return false, nil
		}
		if gatewayID != 0 && contract.GatewayId != gatewayID {
			return false, nil
		}
		total++
		if total <= offset {
			return false, nil
		}
		if uint64(len(collected)) >= limit {
			return true, nil
		}
		ct := contract
		collected = append(collected, &ct)
		return false, nil
	})

	return &types.QueryContractsResponse{
		Contracts: collected,
		Total:     total,
	}, nil
}
