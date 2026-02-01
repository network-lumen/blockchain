package keeper_test

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"lumen/x/tokenomics/keeper"
	"lumen/x/tokenomics/types"
)

func TestUpdateSlashingLivenessParams_SucceedsAndUpdatesParams(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	before, err := f.slash.GetParams(context.Background())
	require.NoError(t, err)

	req := &types.MsgUpdateSlashingLivenessParams{
		Authority:          authority,
		SignedBlocksWindow: 123,
		MinSignedPerWindow: "0.9",
	}

	_, err = server.UpdateSlashingLivenessParams(f.sdkCtx, req)
	require.NoError(t, err)

	after, err := f.slash.GetParams(context.Background())
	require.NoError(t, err)

	require.Equal(t, int64(123), after.SignedBlocksWindow)
	expectedMin, err := sdkmath.LegacyNewDecFromStr("0.9")
	require.NoError(t, err)
	require.True(t, after.MinSignedPerWindow.Equal(expectedMin))

	require.Equal(t, before.DowntimeJailDuration, after.DowntimeJailDuration)
	require.Equal(t, before.SlashFractionDowntime, after.SlashFractionDowntime)
	require.Equal(t, before.SlashFractionDoubleSign, after.SlashFractionDoubleSign)
}

func TestUpdateSlashingLivenessParams_InvalidAuthorityFails(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	otherAuthority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		authtypes.NewModuleAddress("not-gov"),
	)

	before, err := f.slash.GetParams(context.Background())
	require.NoError(t, err)

	req := &types.MsgUpdateSlashingLivenessParams{
		Authority:          otherAuthority,
		SignedBlocksWindow: 123,
		MinSignedPerWindow: "0.9",
	}

	_, err = server.UpdateSlashingLivenessParams(f.sdkCtx, req)
	require.Error(t, err)

	after, err := f.slash.GetParams(context.Background())
	require.NoError(t, err)
	require.Equal(t, before, after)
}
