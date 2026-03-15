package app

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	sdklog "cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
)

func TestAppInitializesIBCIntegration(t *testing.T) {
	appOpts := make(simtestutil.AppOptionsMap)
	appOpts[flags.FlagHome] = t.TempDir()
	appOpts[server.FlagMinGasPrices] = "0ulmn"

	app := New(sdklog.NewNopLogger(), dbm.NewMemDB(), nil, true, appOpts)
	require.NotNil(t, app.IBCKeeper)
	require.Equal(t, authtypes.NewModuleAddress(govtypes.ModuleName).String(), app.TransferKeeper.GetAuthority())
	require.NotNil(t, app.AuthKeeper.GetModuleAddress(ibctransfertypes.ModuleName))

	_, hasIBC := app.ModuleManager.Modules[ibcexported.ModuleName]
	_, hasTransfer := app.ModuleManager.Modules[ibctransfertypes.ModuleName]
	require.True(t, hasIBC)
	require.True(t, hasTransfer)

	genesis := app.DefaultGenesis()
	require.Contains(t, genesis, ibcexported.ModuleName)
	require.Contains(t, genesis, ibctransfertypes.ModuleName)

	initIdx := slices.Index(app.ModuleManager.OrderInitGenesis, ibcexported.ModuleName)
	transferInitIdx := slices.Index(app.ModuleManager.OrderInitGenesis, ibctransfertypes.ModuleName)
	beginIdx := slices.Index(app.ModuleManager.OrderBeginBlockers, ibcexported.ModuleName)
	govBeginIdx := slices.Index(app.ModuleManager.OrderBeginBlockers, govtypes.ModuleName)

	require.NotEqual(t, -1, initIdx)
	require.NotEqual(t, -1, transferInitIdx)
	require.NotEqual(t, -1, beginIdx)
	require.NotEqual(t, -1, govBeginIdx)
	require.Greater(t, beginIdx, govBeginIdx)
}

func TestIBCModuleAccountPermissionsAreRegistered(t *testing.T) {
	perms, ok := GetMaccPerms()[ibctransfertypes.ModuleName]
	require.True(t, ok)
	require.ElementsMatch(t, []string{authtypes.Minter, authtypes.Burner}, perms)

	require.True(t, BlockedAddresses()[ibctransfertypes.ModuleName])
}
