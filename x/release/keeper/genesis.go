package keeper

import (
	"context"

	"lumen/x/release/types"
)

func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if genState.Params != nil {
		if err := k.Params.Set(ctx, *genState.Params); err != nil {
			return err
		}
	}
	for _, r := range genState.Releases {
		if err := k.Release.Set(ctx, r.Id, *r); err != nil {
			return err
		}
		if err := k.ByVersion.Set(ctx, r.Version, r.Id); err != nil {
			return err
		}
		if !r.Yanked && r.Status == types.Release_VALIDATED {
			for _, a := range r.Artifacts {
				if a == nil {
					continue
				}
				if err := k.ByTriple.Set(ctx, tripleKey(r.Channel, a.Platform, a.Kind), r.Id); err != nil {
					return err
				}
			}
		}
	}
	if genState.BundleCount > 0 {
		if err := k.ReleaseSeq.Set(ctx, genState.BundleCount); err != nil {
			return err
		}
	} else {
		var max uint64
		_ = k.Release.Walk(ctx, nil, func(id uint64, _ types.Release) (bool, error) {
			if id > max {
				max = id
			}
			return false, nil
		})
		if err := k.ReleaseSeq.Set(ctx, max); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	genesis := types.DefaultGenesis()

	p, err := k.Params.Get(ctx)
	if err == nil {
		genesis.Params = &p
	}

	list := []*types.Release{}
	_ = k.Release.Walk(ctx, nil, func(_ uint64, r types.Release) (bool, error) { rr := r; list = append(list, &rr); return false, nil })
	genesis.Releases = list

	last, _ := k.ReleaseSeq.Peek(ctx)
	genesis.BundleCount = last

	return genesis, nil
}
