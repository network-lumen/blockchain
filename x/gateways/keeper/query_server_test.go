package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"lumen/x/gateways/keeper"
	"lumen/x/gateways/types"
)

func TestGatewaysQueryClamp(t *testing.T) {
	f := initGatewayFixture(t)
	q := keeper.NewQueryServerImpl(f.keeper)

	for i := uint64(1); i <= 300; i++ {
		err := f.keeper.Gateways.Set(f.ctx, i, types.Gateway{
			Id:       i,
			Operator: randomAccAddress(),
			Active:   true,
		})
		require.NoError(t, err)
	}

	resp, err := q.Gateways(f.ctx, &types.QueryGatewaysRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Gateways, int(keeper.DefaultQueryLimit))

	resp, err = q.Gateways(f.ctx, &types.QueryGatewaysRequest{Limit: 1_000})
	require.NoError(t, err)
	require.Len(t, resp.Gateways, int(keeper.MaxQueryLimit))
}

func TestContractsQueryClamp(t *testing.T) {
	f := initGatewayFixture(t)
	q := keeper.NewQueryServerImpl(f.keeper)

	for i := uint64(1); i <= 250; i++ {
		err := f.keeper.Contracts.Set(f.ctx, i, types.Contract{
			Id:        i,
			Client:    randomAccAddress(),
			GatewayId: 1,
			Status:    types.ContractStatus_CONTRACT_STATUS_ACTIVE,
		})
		require.NoError(t, err)
	}

	resp, err := q.Contracts(f.ctx, &types.QueryContractsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Contracts, int(keeper.DefaultQueryLimit))

	resp, err = q.Contracts(f.ctx, &types.QueryContractsRequest{Limit: 5_000})
	require.NoError(t, err)
	require.Len(t, resp.Contracts, int(keeper.MaxQueryLimit))
}
