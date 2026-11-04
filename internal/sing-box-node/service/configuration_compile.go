// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

type ConfigurationCompileRequest struct {
	CoreArtifactID string `json:"core_artifact_id"`
}

type CompiledConfigurationArtifact struct {
	ID                  string                     `json:"id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	State               store.StartupArtifactState `json:"state"`
}

// CompileConfiguration snapshots the latest configuration the panel sent and
// validates it with the selected exact binary.
func (application *Node) CompileConfiguration(
	ctx context.Context,
	request ConfigurationCompileRequest,
) (CompiledConfigurationArtifact, error) {
	preview, err := application.preview(ctx, request.CoreArtifactID)
	if err != nil {
		return CompiledConfigurationArtifact{}, err
	}
	startupID, err := database.NewID(application.random, "startup")
	if err != nil {
		return CompiledConfigurationArtifact{}, err
	}
	stored, err := application.database.CreateCompiledStartupArtifact(ctx, store.StartupArtifact{
		ID: startupID, CanonicalRevisionID: preview.CanonicalRevision.ID,
		ExactCoreVersion: preview.CoreArtifact.ExactVersion,
		CoreArtifactID:   preview.CoreArtifact.ID, ConfigBytes: preview.Config,
		CreatedAt: application.now().UTC(),
	}, store.CompiledStartupEvidence{
		ExpectedCanonicalHeadID: preview.CanonicalRevision.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrCompiledStartupEvidenceStale) {
			return CompiledConfigurationArtifact{}, fmt.Errorf("configuration changed while compiling: %w", err)
		}
		return CompiledConfigurationArtifact{}, err
	}
	checked, err := application.CheckStartup(ctx, stored.ID)
	if err != nil {
		return CompiledConfigurationArtifact{}, err
	}
	return CompiledConfigurationArtifact{
		ID: stored.ID, CanonicalRevisionID: stored.CanonicalRevisionID,
		ExactCoreVersion: stored.ExactCoreVersion, CoreArtifactID: stored.CoreArtifactID,
		ConfigSHA256: stored.ConfigSHA256, State: checked.State,
	}, nil
}
