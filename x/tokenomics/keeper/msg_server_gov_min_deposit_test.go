package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/require"

	"lumen/x/tokenomics/keeper"
	"lumen/x/tokenomics/types"
)

func TestUpdateGovMinDeposit_OnlyMutatesMinDeposit(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	base := govv1.DefaultParams()
	base.MaxDepositPeriod = durationPtr(48 * time.Hour)
	base.VotingPeriod = durationPtr(48 * time.Hour)
	require.NoError(t, f.gov.SetParams(f.sdkCtx, base))

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	_, err := server.UpdateGovMinDeposit(f.sdkCtx, &types.MsgUpdateGovMinDeposit{
		Authority:  authority,
		MinDeposit: sdk.NewCoins(sdk.NewInt64Coin("ulmn", 15_000_000)),
	})
	require.NoError(t, err)

	stored, err := f.gov.GetParams(f.sdkCtx)
	require.NoError(t, err)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 15_000_000)), sdk.NewCoins(stored.MinDeposit...))
	require.Equal(t, base.Quorum, stored.Quorum)
	require.Equal(t, base.Threshold, stored.Threshold)
	require.Equal(t, base.MaxDepositPeriod, stored.MaxDepositPeriod)
	require.Equal(t, base.VotingPeriod, stored.VotingPeriod)
}

func TestUpdateGovMinDeposit_InvalidRequest(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	_, err := server.UpdateGovMinDeposit(f.sdkCtx, &types.MsgUpdateGovMinDeposit{
		Authority:  authority,
		MinDeposit: nil,
	})
	require.ErrorContains(t, err, "min_deposit must be set")
}

func TestUpdateGovMinDeposit_InvalidAuthority(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	_, err := server.UpdateGovMinDeposit(f.sdkCtx, &types.MsgUpdateGovMinDeposit{
		Authority:  authtypes.NewModuleAddress("not-gov").String(),
		MinDeposit: sdk.NewCoins(sdk.NewInt64Coin("ulmn", 10_000_000)),
	})
	require.ErrorContains(t, err, "invalid authority")
}

func TestUpdateGovMinDeposit_RejectsOutOfBoundsOrWrongDenom(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	testCases := []struct {
		name       string
		minDeposit sdk.Coins
		errMsg     string
	}{
		{
			name:       "wrong denom",
			minDeposit: sdk.NewCoins(sdk.NewInt64Coin("uatom", 10_000_000)),
			errMsg:     "denom must be ulmn",
		},
		{
			name:       "above cap",
			minDeposit: sdk.NewCoins(sdk.NewCoin("ulmn", sdkmath.NewInt(1_000_000_000_001))),
			errMsg:     "must be <=",
		},
		{
			name:       "multi coin",
			minDeposit: sdk.NewCoins(sdk.NewInt64Coin("foo", 1), sdk.NewInt64Coin("ulmn", 10_000_000)),
			errMsg:     "exactly one coin",
		},
		{
			name:       "zero amount",
			minDeposit: sdk.Coins{sdk.Coin{Denom: "ulmn", Amount: sdkmath.ZeroInt()}},
			errMsg:     "min_deposit must be set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server.UpdateGovMinDeposit(f.sdkCtx, &types.MsgUpdateGovMinDeposit{
				Authority:  authority,
				MinDeposit: tc.minDeposit,
			})
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
