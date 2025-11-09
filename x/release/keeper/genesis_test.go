//go:build legacy

package keeper_test

import (
	"testing"

	"lumen/x/release/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	p := types.DefaultParams()
	genesisState := types.GenesisState{
		Params:      &p,
		Releases:    []*types.Release{{Id: 1}, {Id: 2}},
		BundleCount: 2,
	}
	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.Releases, got.Releases)
	require.Equal(t, genesisState.BundleCount, got.BundleCount)

}

