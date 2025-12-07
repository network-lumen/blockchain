package keeper_test

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"lumen/x/tokenomics/keeper"
	"lumen/x/tokenomics/types"
)

func TestUpdateSlashingDowntimeParams_SucceedsAndUpdatesParams(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)

	// sanity: we have slashing params via mock adapter
	before, err := f.slash.GetParams(context.Background())
	require.NoError(t, err)

	req := &types.MsgUpdateSlashingDowntimeParams{
		Authority:             authority,
		SlashFractionDowntime: "0.02",
		DowntimeJailDuration:  "600s",
	}

	_, err = server.UpdateSlashingDowntimeParams(f.sdkCtx, req)
	require.NoError(t, err)

	after, err := f.slash.GetParams(context.Background())
	require.NoError(t, err)

	require.Equal(t, before.SlashFractionDoubleSign, after.SlashFractionDoubleSign, "double-sign fraction must remain unchanged")
	expectedFrac, err := sdkmath.LegacyNewDecFromStr("0.02")
	require.NoError(t, err)
	require.True(t, after.SlashFractionDowntime.Equal(expectedFrac))
	require.Equal(t, time.Second*600, after.DowntimeJailDuration)
}
