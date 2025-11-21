package keeper_test

import (
	"testing"

	"lumen/x/dns/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params: func() types.Params {
			return types.DefaultParams()
		}(),
		DomainMap: []types.Domain{{Index: "0"}, {Index: "1"}}, AuctionMap: []types.Auction{{Index: "0"}, {Index: "1"}}}

	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.DomainMap, got.DomainMap)
	require.EqualExportedValues(t, genesisState.AuctionMap, got.AuctionMap)

}
