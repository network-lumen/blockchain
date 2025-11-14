package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"lumen/x/tokenomics/keeper"
	"lumen/x/tokenomics/types"
)

func TestUpdateParamsMutableFields(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	base := f.keeper.GetParams(f.sdkCtx)
	updated := base
	updated.TxTaxRate = "0.02"
	updated.MinSendUlmn = base.MinSendUlmn * 2

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	_, err := server.UpdateParams(f.sdkCtx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    updated,
	})
	require.NoError(t, err)

	stored := f.keeper.GetParams(f.sdkCtx)
	require.Equal(t, updated, stored)
}

func TestUpdateParamsImmutableFields(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)
	base := f.keeper.GetParams(f.sdkCtx)
	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	testCases := []struct {
		name string
		mut  func(p *types.Params)
	}{
		{"denom", func(p *types.Params) { p.Denom = "uatom" }},
		{"decimals", func(p *types.Params) { p.Decimals = base.Decimals + 1 }},
		{"supply_cap", func(p *types.Params) { p.SupplyCapLumn = base.SupplyCapLumn + 1 }},
		{"halving_interval", func(p *types.Params) { p.HalvingIntervalBlocks = base.HalvingIntervalBlocks + 10 }},
		{"initial_reward", func(p *types.Params) { p.InitialRewardPerBlockLumn = base.InitialRewardPerBlockLumn + 1 }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updated := base
			tc.mut(&updated)

			_, err := server.UpdateParams(f.sdkCtx, &types.MsgUpdateParams{
				Authority: authority,
				Params:    updated,
			})
			require.ErrorContains(t, err, "immutable")
		})
	}
}
