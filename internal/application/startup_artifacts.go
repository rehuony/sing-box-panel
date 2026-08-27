// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// StartupArtifactSummary deliberately excludes startup configuration bytes.
// Those bytes can contain credentials and are only exposed through the
// explicit manual-artifact read boundary.
type StartupArtifactSummary struct {
	ID                  string                     `json:"id"`
	Kind                store.StartupArtifactKind  `json:"kind"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	CapabilityCommit    string                     `json:"capability_commit,omitempty"`
	CapabilityDigest    string                     `json:"capability_digest,omitempty"`
	RendererVersion     string                     `json:"renderer_version"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	Diagnostics         json.RawMessage            `json:"diagnostics"`
	State               store.StartupArtifactState `json:"state"`
	CheckedAt           *time.Time                 `json:"checked_at,omitempty"`
	CreatedAt           time.Time                  `json:"created_at"`
}

type StartupArtifactCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type StartupArtifactListRequest struct {
	CanonicalRevisionID string
	ExactCoreVersion    string
	CoreArtifactID      string
	Kind                store.StartupArtifactKind
	State               store.StartupArtifactState
	Cursor              *StartupArtifactCursor
	Limit               int
}

type StartupArtifactPage struct {
	Items []StartupArtifactSummary `json:"items"`
	Next  *StartupArtifactCursor   `json:"next,omitempty"`
}

func (application *Application) ListStartupArtifacts(
	ctx context.Context,
	request StartupArtifactListRequest,
) (StartupArtifactPage, error) {
	request.ExactCoreVersion = strings.TrimSpace(request.ExactCoreVersion)
	if request.ExactCoreVersion != "" {
		version, err := coreartifact.ParseExactVersion(request.ExactCoreVersion)
		if err != nil || version.IsZero() {
			return StartupArtifactPage{}, fmt.Errorf("invalid exact core version %q", request.ExactCoreVersion)
		}
		request.ExactCoreVersion = version.String()
	}
	var cursor *store.CreatedAtCursor
	if request.Cursor != nil {
		cursor = &store.CreatedAtCursor{CreatedAt: request.Cursor.CreatedAt, ID: strings.TrimSpace(request.Cursor.ID)}
	}
	page, err := application.database.ListStartupArtifacts(ctx, store.StartupArtifactListFilter{
		CanonicalRevisionID: strings.TrimSpace(request.CanonicalRevisionID),
		ExactCoreVersion:    request.ExactCoreVersion,
		CoreArtifactID:      strings.TrimSpace(request.CoreArtifactID),
		Kind:                request.Kind,
		State:               request.State,
		Cursor:              cursor,
		Limit:               request.Limit,
	})
	if err != nil {
		return StartupArtifactPage{}, err
	}
	result := StartupArtifactPage{Items: make([]StartupArtifactSummary, len(page.Items))}
	for index, artifact := range page.Items {
		result.Items[index] = startupArtifactSummary(artifact)
	}
	if page.Next != nil {
		result.Next = &StartupArtifactCursor{CreatedAt: page.Next.CreatedAt, ID: page.Next.ID}
	}
	return result, nil
}

func startupArtifactSummary(artifact store.StartupArtifactSummary) StartupArtifactSummary {
	return StartupArtifactSummary{
		ID: artifact.ID, Kind: artifact.Kind, CanonicalRevisionID: artifact.CanonicalRevisionID,
		ExactCoreVersion: artifact.ExactCoreVersion, CapabilityCommit: artifact.CapabilityCommit,
		CapabilityDigest: artifact.CapabilityDigest, RendererVersion: artifact.RendererVersion,
		CoreArtifactID: artifact.CoreArtifactID, ConfigSHA256: artifact.ConfigSHA256,
		Diagnostics: append(json.RawMessage(nil), artifact.Diagnostics...), State: artifact.State,
		CheckedAt: artifact.CheckedAt, CreatedAt: artifact.CreatedAt,
	}
}
