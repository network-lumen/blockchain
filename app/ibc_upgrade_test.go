package app

import (
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/rootmulti"
	storetypes "cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
)

func TestIBCStoresMissingAtLatestVersion(t *testing.T) {
	t.Run("missing ibc stores", func(t *testing.T) {
		db := dbm.NewMemDB()
		store := rootmulti.NewStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
		store.MountStoreWithDB(storetypes.NewKVStoreKey("auth"), storetypes.StoreTypeIAVL, nil)
		store.MountStoreWithDB(storetypes.NewKVStoreKey("bank"), storetypes.StoreTypeIAVL, nil)
		require.NoError(t, store.LoadLatestVersion())
		store.Commit()

		missing, err := ibcStoresMissingAtLatestVersion(db)
		require.NoError(t, err)
		require.True(t, missing)
	})

	t.Run("ibc stores already present", func(t *testing.T) {
		db := dbm.NewMemDB()
		store := rootmulti.NewStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
		store.MountStoreWithDB(storetypes.NewKVStoreKey("auth"), storetypes.StoreTypeIAVL, nil)
		store.MountStoreWithDB(storetypes.NewKVStoreKey(ibcexported.StoreKey), storetypes.StoreTypeIAVL, nil)
		store.MountStoreWithDB(storetypes.NewKVStoreKey(ibctransfertypes.StoreKey), storetypes.StoreTypeIAVL, nil)
		require.NoError(t, store.LoadLatestVersion())
		store.Commit()

		missing, err := ibcStoresMissingAtLatestVersion(db)
		require.NoError(t, err)
		require.False(t, missing)
	})
}

func TestIsIBCCompatibleUpgradePlan(t *testing.T) {
	require.True(t, isIBCCompatibleUpgradePlan("v1.5.0"))
	require.True(t, isIBCCompatibleUpgradePlan("v1.5.2"))
	require.False(t, isIBCCompatibleUpgradePlan("v1.4.3"))
}
