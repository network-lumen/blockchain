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
		// No-op handler: keep the existing version map and continue.
		return fromVM, nil
	})
}
