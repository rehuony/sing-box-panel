// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
)

// SyncEnabledCoreLink projects committed selection into the filesystem. It is
// repeated at startup to repair a crash between the SQLite commit and rename.
func (application *Node) SyncEnabledCoreLink(ctx context.Context, artifacts *artifacts.Store) error {
	bootstrap, err := application.database.Bootstrap(ctx)
	if err != nil {
		return err
	}
	if bootstrap.Hub.AppliedBundleID == "" {
		return artifacts.SetCurrent(ctx, "")
	}
	material, err := application.LoadRuntimeMaterial(ctx, bootstrap.Hub.AppliedBundleID)
	if err != nil {
		return err
	}
	return artifacts.SetCurrent(ctx, material.Core.BinaryPath)
}
