package types_test

import (
	"testing"

	"lumen/x/dns/types"

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
			desc: "valid genesis state",
			genState: &types.GenesisState{
				Params:     types.DefaultParams(),
				DomainMap:  []types.Domain{{Index: "0"}, {Index: "1"}},
				AuctionMap: []types.Auction{{Index: "0"}, {Index: "1"}},
			},
			valid: true,
		}, {
			desc: "duplicated domain",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				DomainMap: []types.Domain{
					{
						Index: "0",
					},
					{
						Index: "0",
					},
				},
				AuctionMap: []types.Auction{{Index: "0"}, {Index: "1"}},
			},
			valid: false,
		}, {
			desc: "duplicated auction",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				AuctionMap: []types.Auction{
					{
						Index: "0",
					},
					{
						Index: "0",
					},
				},
			},
			valid: false,
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
