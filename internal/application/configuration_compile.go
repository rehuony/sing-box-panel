// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type ConfigurationCompileRequest struct {
	CoreArtifactID        string `json:"core_artifact_id"`
	AcceptedIgnoredDigest string `json:"accepted_ignored_digest,omitempty"`
}

type CompiledConfigurationArtifact struct {
	ID                  string                     `json:"id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	AdapterID           string                     `json:"adapter_id"`
	AdapterRevision     string                     `json:"adapter_revision"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	Diagnostics         []adapter.Diagnostic       `json:"diagnostics"`
	IgnoredDigest       string                     `json:"ignored_digest,omitempty"`
	State               store.StartupArtifactState `json:"state"`
}

type ConfigurationCompile struct {
	Support  ConfigurationAdapterSupport   `json:"support"`
	Artifact CompiledConfigurationArtifact `json:"artifact"`
	Task     Task                          `json:"task"`
}

// CompileConfiguration projects the current global revision through the exact
// reviewed adapter and atomically queues validation by the selected binary.
func (application *Application) CompileConfiguration(
	ctx context.Context,
	request ConfigurationCompileRequest,
) (ConfigurationCompile, error) {
	preview, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{
		CoreArtifactID: strings.TrimSpace(request.CoreArtifactID),
	})
	if err != nil {
		return ConfigurationCompile{}, err
	}
	projection := adapter.Result{
		ConfigJSON: preview.Config, Diagnostics: preview.Diagnostics, IgnoredDigest: preview.IgnoredDigest,
	}
	if err := adapter.RequireIgnoredAcceptance(projection, request.AcceptedIgnoredDigest); err != nil {
		return ConfigurationCompile{}, err
	}
	diagnosticsJSON, err := json.Marshal(preview.Diagnostics)
	if err != nil {
		return ConfigurationCompile{}, fmt.Errorf("encode projection diagnostics: %w", err)
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return ConfigurationCompile{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return ConfigurationCompile{}, err
	}
	createdAt := application.now().UTC()
	payload, err := json.Marshal(map[string]string{"startup_artifact_id": startupID})
	if err != nil {
		return ConfigurationCompile{}, err
	}
	stored, err := application.database.CreateStartupArtifactAndCheckTask(ctx, store.StartupArtifact{
		ID: startupID, CanonicalRevisionID: preview.CanonicalRevision.ID,
		ExactCoreVersion: preview.CoreArtifact.ExactVersion,
		AdapterID:        preview.Support.AdapterID, AdapterRevision: preview.Support.Revision,
		CoreArtifactID: preview.CoreArtifact.ID, ConfigBytes: preview.Config,
		Diagnostics: diagnosticsJSON, IgnoredDigest: preview.IgnoredDigest, CreatedAt: createdAt,
	}, store.NewTask{
		ID: taskID, IdempotencyKey: "startup-check:" + startupID,
		Lane: store.TaskLaneMaintenance, Kind: "startup-check", Payload: payload, CreatedAt: createdAt,
	}, store.CompiledStartupEvidence{
		ExpectedCanonicalHeadID: preview.CanonicalRevision.ID,
		AdapterID:               preview.Support.AdapterID, AdapterRevision: preview.Support.Revision,
	})
	if err != nil {
		if errors.Is(err, store.ErrCompiledStartupEvidenceStale) {
			return ConfigurationCompile{}, fmt.Errorf("configuration changed while compiling: %w", err)
		}
		return ConfigurationCompile{}, err
	}
	return ConfigurationCompile{
		Support: preview.Support,
		Artifact: CompiledConfigurationArtifact{
			ID: stored.Artifact.ID, CanonicalRevisionID: stored.Artifact.CanonicalRevisionID,
			ExactCoreVersion: stored.Artifact.ExactCoreVersion, AdapterID: stored.Artifact.AdapterID,
			AdapterRevision: stored.Artifact.AdapterRevision, CoreArtifactID: stored.Artifact.CoreArtifactID,
			ConfigSHA256:  stored.Artifact.ConfigSHA256,
			Diagnostics:   append([]adapter.Diagnostic(nil), preview.Diagnostics...),
			IgnoredDigest: stored.Artifact.IgnoredDigest, State: stored.Artifact.State,
		},
		Task: applicationTask(stored.Task),
	}, nil
}
