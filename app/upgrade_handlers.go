package app

import (
	"context"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// RegisterUpgradeHandlers installs upgrade handlers for named plans.
func (app *App) RegisterUpgradeHandlers() {
	if app.UpgradeKeeper == nil {
		return
	}

	app.UpgradeKeeper.SetUpgradeHandler("v1", func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Legacy handler: kept for backwards compatibility, but still run migrations so
		// module versions are updated and invariants stay consistent across upgrades.
		return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
	})

	app.UpgradeKeeper.SetUpgradeHandler("v1.4.2", func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// No-op state transition, but still run migrations so module versions are updated.
		return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
	})
}
