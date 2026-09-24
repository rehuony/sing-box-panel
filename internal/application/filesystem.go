// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/filesystem"
)

// Core path selection uses the same working directory as runtime.Manager.
func (application *Application) ListFilesystemEntries(ctx context.Context, query filesystem.Query) (filesystem.Page, error) {
	return filesystem.List(ctx, filepath.Join(application.settings.DataDir, "runtime"), query)
}

func (application *Application) ResolveFilesystemPath(ctx context.Context, path string, mode filesystem.Mode) (filesystem.Selection, error) {
	return filesystem.Resolve(ctx, filepath.Join(application.settings.DataDir, "runtime"), path, mode)
}
