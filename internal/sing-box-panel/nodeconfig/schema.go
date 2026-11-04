// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/schema"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

type ConfigurationSupport struct {
	Structured   bool   `json:"structured"`
	ExactVersion string `json:"exact_version"`
	Reason       string `json:"reason,omitempty"`
}

type ConfigurationSchema struct {
	ExactVersion string          `json:"exact_version"`
	SchemaSHA256 string          `json:"schema_sha256"`
	Schema       json.RawMessage `json:"schema"`
}

type ConfigurationPreviewRequest struct {
	CoreArtifactID string `json:"core_artifact_id"`
}

type ConfigurationPreview struct {
	CanonicalRevision CanonicalSnapshot      `json:"canonical_revision"`
	CoreArtifact      corecache.CoreArtifact `json:"core_artifact"`
	Support           ConfigurationSupport   `json:"support"`
	Config            json.RawMessage        `json:"config"`
}

func (application *Service) configurationSupport(core store.CachedArtifact) ConfigurationSupport {
	_, err := schema.ConfigurationSchema(core.ExactVersion)
	if err != nil {
		return ConfigurationSupport{
			Structured:   false,
			ExactVersion: core.ExactVersion,
			Reason:       err.Error(),
		}
	}
	return ConfigurationSupport{
		Structured:   true,
		ExactVersion: core.ExactVersion,
	}
}

// ConfigurationSupport reports whether the exact sing-box version has a committed
// browser schema. Raw JSON editing, checking, and execution do not depend on
// this presentation capability.
func (application *Service) ConfigurationSupport(
	ctx context.Context,
	coreArtifactID string,
) (ConfigurationSupport, error) {
	core, err := application.cores.CachedArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return ConfigurationSupport{}, err
	}
	return application.configurationSupport(core), nil
}

// ConfigurationSchema returns the canonical browser schema selected only
// by the artifact's exact sing-box version.
func (application *Service) ConfigurationSchema(
	ctx context.Context,
	coreArtifactID string,
) (ConfigurationSchema, error) {
	core, err := application.cores.CachedArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return ConfigurationSchema{}, err
	}
	contract, err := schema.ConfigurationSchema(core.ExactVersion)
	if err != nil {
		return ConfigurationSchema{}, err
	}
	return ConfigurationSchema{
		ExactVersion: core.ExactVersion,
		SchemaSHA256: contract.SchemaSHA256,
		Schema:       append(json.RawMessage(nil), contract.Schema...),
	}, nil
}

// PreviewConfiguration snapshots nodeID's current valid file for the selected
// core without persisting startup bytes.
func (application *Service) PreviewConfiguration(
	ctx context.Context,
	nodeID string,
	request ConfigurationPreviewRequest,
) (ConfigurationPreview, error) {
	core, err := application.cores.CachedArtifact(ctx, strings.TrimSpace(request.CoreArtifactID))
	if err != nil {
		return ConfigurationPreview{}, err
	}
	revision, _, err := application.CurrentRevision(ctx, nodeID)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	document, err := configuration.Parse(revision.Document)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	return ConfigurationPreview{
		CanonicalRevision: Snapshot(revision),
		CoreArtifact: corecache.CoreArtifact{
			ID: core.ID, ExactVersion: core.ExactVersion, OperatingSystem: core.OS, Architecture: core.Arch,
			Variant: core.Variant, SourceKind: core.SourceKind, UserSource: core.UserSource,
			RepositoryID: core.RepositoryID, ReleaseID: core.ReleaseID, AssetID: core.AssetID,
			ArchiveSHA256: core.SHA256, ReportedVersion: core.ExactVersion, FeatureFingerprint: json.RawMessage(`{}`), CreatedAt: core.CreatedAt,
		},
		Support: application.configurationSupport(core),
		Config:  append(json.RawMessage(nil), document.CanonicalJSON()...),
	}, nil
}

// ValidateForCore checks a revision against the exact Schema of the core,
// when the panel has one, before the revision reaches a node.
func (application *Service) ValidateForCore(ctx context.Context, revision store.CanonicalRevision, coreArtifactID string) error {
	core, err := application.cores.CachedArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return err
	}
	if !application.configurationSupport(core).Structured {
		return nil
	}
	if err := schema.ValidateConfiguration(core.ExactVersion, revision.Document); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigurationSchemaValidation, err)
	}
	return nil
}

// ErrConfigurationSchemaValidation rejects a configuration that the core's
// exact Schema does not accept.
var ErrConfigurationSchemaValidation = errors.New("configuration schema validation failed")
