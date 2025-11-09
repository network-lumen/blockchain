package keeper

import (
	"context"

	"lumen/x/gateways/types"
)

func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if genState.Params != nil {
		if err := k.SetParams(ctx, *genState.Params); err != nil {
			return err
		}
	}

	for _, gw := range genState.Gateways {
		if err := k.setGateway(ctx, *gw); err != nil {
			return err
		}
	}

	for _, ct := range genState.Contracts {
		if err := k.setContract(ctx, *ct); err != nil {
			return err
		}
	}

	if genState.GatewayCount > 0 {
		_ = k.GatewaySeq.Set(ctx, genState.GatewayCount)
	} else {
		var max uint64
		_ = k.Gateways.Walk(ctx, nil, func(id uint64, _ types.Gateway) (bool, error) {
			if id > max {
				max = id
			}
			return false, nil
		})
		_ = k.GatewaySeq.Set(ctx, max)
	}

	if genState.ContractCount > 0 {
		_ = k.ContractSeq.Set(ctx, genState.ContractCount)
	} else {
		var max uint64
		_ = k.Contracts.Walk(ctx, nil, func(id uint64, _ types.Contract) (bool, error) {
			if id > max {
				max = id
			}
			return false, nil
		})
		_ = k.ContractSeq.Set(ctx, max)
	}

	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	genesis := types.DefaultGenesis()

	if params, err := k.Params.Get(ctx); err == nil {
		genesis.Params = &params
	}

	gateways := make([]*types.Gateway, 0)
	_ = k.Gateways.Walk(ctx, nil, func(_ uint64, gw types.Gateway) (bool, error) {
		g := gw
		gateways = append(gateways, &g)
		return false, nil
	})
	genesis.Gateways = gateways

	contracts := make([]*types.Contract, 0)
	_ = k.Contracts.Walk(ctx, nil, func(_ uint64, ct types.Contract) (bool, error) {
		c := ct
		contracts = append(contracts, &c)
		return false, nil
	})
	genesis.Contracts = contracts

	lastGateway, _ := k.GatewaySeq.Peek(ctx)
	lastContract, _ := k.ContractSeq.Peek(ctx)

	genesis.GatewayCount = lastGateway
	genesis.ContractCount = lastContract

	return genesis, nil
}
