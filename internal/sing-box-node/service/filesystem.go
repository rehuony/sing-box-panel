// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
)

// Core path selection uses the same working directory as runtime.Manager.
func (application *Node) ListFilesystemEntries(ctx context.Context, query host.PathQuery) (host.PathPage, error) {
	return host.ListPaths(ctx, filepath.Join(application.dataDir, "runtime"), query)
}

func (application *Node) ResolveFilesystemPath(ctx context.Context, path string, mode host.PathMode) (host.PathSelection, error) {
	return host.ResolvePath(ctx, filepath.Join(application.dataDir, "runtime"), path, mode)
}

// Path selection and core log types the management API exchanges with the node.
type (
	PathQuery     = host.PathQuery
	PathPage      = host.PathPage
	PathMode      = host.PathMode
	PathSelection = host.PathSelection
	CoreLogFile   = corelogs.File
	CoreLogChunk  = corelogs.Chunk
)

// Path selection and core log failures the management API maps to problems.
var (
	ErrInvalidPath       = host.ErrInvalidPath
	ErrBrokenLink        = host.ErrBrokenLink
	ErrPathType          = host.ErrPathType
	ErrDirectoryTooLarge = host.ErrDirectoryTooLarge
)
