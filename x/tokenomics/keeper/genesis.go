package keeper

import (
	"fmt"

	"lumen/x/tokenomics/types"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	params := gs.Params
	if params == nil {
		def := types.DefaultParams()
		params = &def
	}
	if err := types.ValidateParams(*params); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, *params); err != nil {
		return err
	}
	total := sdkmath.ZeroInt()
	if gs.TotalMintedUlmn != "" {
		amt, ok := sdkmath.NewIntFromString(gs.TotalMintedUlmn)
		if !ok {
			return fmt.Errorf("invalid total_minted_ulmn: %s", gs.TotalMintedUlmn)
		}
		total = amt
	}
	return k.SetTotalMintedUlmn(ctx, total)
}

func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	params := k.GetParams(ctx)
	amount := k.GetTotalMintedUlmn(ctx)
	return &types.GenesisState{Params: &params, TotalMintedUlmn: amount.String()}, nil
}
