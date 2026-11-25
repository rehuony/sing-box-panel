// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/singbox"
)

var (
	// ErrCoreSchemaUnavailable refuses a core whose release line does not
	// print its configuration schema.
	ErrCoreSchemaUnavailable = errors.New("the core's release line does not print its configuration schema")
	// ErrCoreSchemaFailed reports a core that did not print its schema
	// within the node's limits.
	ErrCoreSchemaFailed = errors.New("the core did not print its configuration schema")
)

// coreSchemaTimeout bounds one run of `sing-box schema`.
const coreSchemaTimeout = 10 * time.Second

// CoreSchema returns the configuration JSON Schema that the installed core
// artifactID prints with `sing-box schema`, at most maximumBytes long. The
// node keeps no copy.
func (application *Node) CoreSchema(ctx context.Context, artifactID string, maximumBytes int64) ([]byte, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(artifactID))
	if err != nil {
		return nil, err
	}
	if line, found := singbox.LineOf(core.ExactVersion); !found || !line.NativeSchema {
		return nil, ErrCoreSchemaUnavailable
	}
	schema, err := artifacts.ReadSchema(ctx, core.BinaryPath, maximumBytes, coreSchemaTimeout)
	if err != nil {
		return nil, errors.Join(ErrCoreSchemaFailed, err)
	}
	return schema, nil
}
