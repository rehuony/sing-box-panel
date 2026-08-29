// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// StartupArtifactSummary deliberately excludes compiled configuration bytes,
// which may contain credentials.
type StartupArtifactSummary struct {
	ID                  string                     `json:"id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	AdapterID           string                     `json:"adapter_id"`
	AdapterRevision     string                     `json:"adapter_revision"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	Diagnostics         json.RawMessage            `json:"diagnostics"`
	IgnoredDigest       string                     `json:"ignored_digest,omitempty"`
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
		ID: artifact.ID, CanonicalRevisionID: artifact.CanonicalRevisionID,
		ExactCoreVersion: artifact.ExactCoreVersion, AdapterID: artifact.AdapterID,
		AdapterRevision: artifact.AdapterRevision,
		CoreArtifactID:  artifact.CoreArtifactID, ConfigSHA256: artifact.ConfigSHA256,
		Diagnostics: append(json.RawMessage(nil), artifact.Diagnostics...), State: artifact.State,
		IgnoredDigest: artifact.IgnoredDigest, CheckedAt: artifact.CheckedAt, CreatedAt: artifact.CreatedAt,
	}
}

func (application *Application) QueueStartupCheck(ctx context.Context, startupArtifactID string) (Task, error) {
	artifact, err := application.database.GetStartupArtifact(ctx, strings.TrimSpace(startupArtifactID))
	if err != nil {
		return Task{}, err
	}
	if artifact.State != store.StartupArtifactPending {
		return Task{}, store.ErrStartupArtifactState
	}
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	payload, err := json.Marshal(map[string]string{"startup_artifact_id": artifact.ID})
	if err != nil {
		return Task{}, err
	}
	queued, err := application.database.EnqueueTask(ctx, store.EnqueueTaskInput{
		ID: taskID, IdempotencyKey: "startup-check:" + artifact.ID,
		Lane: store.TaskLaneMaintenance, Kind: store.TaskKindStartupCheck,
		CanonicalRevisionID: artifact.CanonicalRevisionID, StartupArtifactID: artifact.ID,
		Payload: payload, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(queued), nil
}

func IsStartupArtifactNotFound(err error) bool {
	return errors.Is(err, store.ErrStartupArtifactNotFound)
}
