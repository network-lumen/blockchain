package keeper

import (
	"context"

	"lumen/x/dns/types"
)

func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if genState.Params.BaseFeeDns == "" {
		genState.Params = types.DefaultParams()
	}
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	for _, elem := range genState.DomainMap {
		if err := k.Domain.Set(ctx, elem.Index, elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.AuctionMap {
		if err := k.Auction.Set(ctx, elem.Index, elem); err != nil {
			return err
		}
	}

	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	genesis := types.DefaultGenesis()

	params, err := k.Params.Get(ctx)
	if err != nil || params.BaseFeeDns == "" {
		params = types.DefaultParams()
	}
	genesis.Params = params

	if err := k.Domain.Walk(ctx, nil, func(_ string, val types.Domain) (stop bool, err error) {
		genesis.DomainMap = append(genesis.DomainMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Auction.Walk(ctx, nil, func(_ string, val types.Auction) (stop bool, err error) {
		genesis.AuctionMap = append(genesis.AuctionMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return genesis, nil
}
