// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/singbox"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrConfigurationSchemaValidation = errors.New("configuration schema validation failed")

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

type ConfigurationCompile struct {
	Support  ConfigurationSupport          `json:"support"`
	Artifact CompiledConfigurationArtifact `json:"artifact"`
}

// CompileConfiguration snapshots the current raw JSON revision and atomically
// validates it with the selected exact binary.
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
	if preview.Support.Structured {
		if err := singbox.ValidateConfiguration(preview.CoreArtifact.ExactVersion, preview.Config); err != nil {
			return ConfigurationCompile{}, fmt.Errorf("%w: %v", ErrConfigurationSchemaValidation, err)
		}
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return ConfigurationCompile{}, err
	}
	createdAt := application.now().UTC()
	stored, err := application.database.CreateCompiledStartupArtifact(ctx, store.StartupArtifact{
		ID: startupID, CanonicalRevisionID: preview.CanonicalRevision.ID,
		ExactCoreVersion: preview.CoreArtifact.ExactVersion,
		CoreArtifactID:   preview.CoreArtifact.ID, ConfigBytes: preview.Config,
		CreatedAt: createdAt,
	}, store.CompiledStartupEvidence{
		ExpectedCanonicalHeadID: preview.CanonicalRevision.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrCompiledStartupEvidenceStale) {
			return ConfigurationCompile{}, fmt.Errorf("configuration changed while compiling: %w", err)
		}
		return ConfigurationCompile{}, err
	}
	checked, err := application.CheckStartup(ctx, stored.ID)
	if err != nil {
		return ConfigurationCompile{}, err
	}
	stored.State = checked.State
	return ConfigurationCompile{
		Support: preview.Support,
		Artifact: CompiledConfigurationArtifact{
			ID: stored.ID, CanonicalRevisionID: stored.CanonicalRevisionID,
			ExactCoreVersion: stored.ExactCoreVersion, CoreArtifactID: stored.CoreArtifactID,
			ConfigSHA256: stored.ConfigSHA256, State: stored.State,
		},
	}, nil
}
