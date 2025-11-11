package keeper

import (
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/x/pqc/types"
)

func (k Keeper) InitGenesis(ctx sdk.Context, genState types.GenesisState) error {
	if err := genState.Validate(); err != nil {
		return err
	}

	if err := k.SetParams(ctx, genState.Params); err != nil {
		return err
	}

	for _, account := range genState.Accounts {
		addr, err := sdk.AccAddressFromBech32(account.Addr)
		if err != nil {
			return err
		}
		if err := k.SetAccountPQC(ctx, addr, account); err != nil {
			return err
		}
	}

	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	params := k.GetParams(ctx)
	accounts := make([]types.AccountPQC, 0, 16)

	err := k.Accounts.Walk(ctx, nil, func(_ string, value types.AccountPQC) (bool, error) {
		copyHash := append([]byte(nil), value.PubKeyHash...)
		value.PubKeyHash = copyHash
		accounts = append(accounts, value)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Addr < accounts[j].Addr
	})

	return &types.GenesisState{
		Params:   params,
		Accounts: accounts,
	}, nil
}
