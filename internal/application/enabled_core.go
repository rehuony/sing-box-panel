// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
)

// SyncEnabledCoreLink projects committed selection into the filesystem. It is
// repeated at startup to repair a crash between the SQLite commit and rename.
func (application *Application) SyncEnabledCoreLink(ctx context.Context, artifacts *artifactstore.Store) error {
	bootstrap, err := application.database.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Hub.AppliedBundleID == "" {
		return artifacts.SetCurrent(ctx, "", "")
	}
	material, err := application.LoadRuntimeMaterial(ctx, bootstrap.Hub.AppliedBundleID)
	if err != nil {
		return err
	}
	return artifacts.SetCurrent(ctx, material.Core.BinaryPath, material.Core.BinarySHA256)
}
