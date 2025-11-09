package types_test

import (
	"testing"

	"lumen/x/release/types"

	"github.com/stretchr/testify/require"
)

func TestGenesisState_Validate(t *testing.T) {
	tests := []struct {
		desc     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			desc:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			desc:     "valid genesis state",
			genState: &types.GenesisState{Releases: []*types.Release{{Id: 1}, {Id: 2}}, BundleCount: 2}, valid: true,
		}, {
			desc: "duplicated bundle",
			genState: &types.GenesisState{
				Releases: []*types.Release{{Id: 1}, {Id: 1}},
			},
			valid: false,
		}, {
			desc:     "empty params still valid",
			genState: &types.GenesisState{Releases: []*types.Release{}, BundleCount: 0},
			valid:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
