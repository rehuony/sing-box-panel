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
	"github.com/rehuony/sing-box-panel/internal/singbox"
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

// CoreSchemaSource returns the configuration schema that the installed core
// coreArtifactID of nodeID prints, as it printed it.
type CoreSchemaSource func(ctx context.Context, nodeID, coreArtifactID string) ([]byte, error)

func (application *Service) configurationSupport(core store.CachedArtifact) ConfigurationSupport {
	if _, found := singbox.LineOf(core.ExactVersion); !found {
		return ConfigurationSupport{
			Structured:   false,
			ExactVersion: core.ExactVersion,
			Reason:       schemaUnavailable(core.ExactVersion).Error(),
		}
	}
	return ConfigurationSupport{
		Structured:   true,
		ExactVersion: core.ExactVersion,
	}
}

func schemaUnavailable(exactVersion string) error {
	return fmt.Errorf("%w: sing-box %s", schema.ErrConfigurationSchemaUnavailable, exactVersion)
}

// ConfigurationSupport reports whether the core's sing-box version has a
// configuration schema for the structured editor: exactly when the version
// belongs to a supported release line. Raw JSON editing, checking and
// execution do not depend on it.
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

// ConfigurationSchema returns the configuration schema of the core
// coreArtifactID: the reviewed schema of its line, or the schema the core
// prints on nodeID.
func (application *Service) ConfigurationSchema(
	ctx context.Context,
	nodeID, coreArtifactID string,
) (ConfigurationSchema, error) {
	core, err := application.cores.CachedArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return ConfigurationSchema{}, err
	}
	contract, err := application.contract(ctx, nodeID, core)
	if err != nil {
		return ConfigurationSchema{}, err
	}
	return ConfigurationSchema{
		ExactVersion: core.ExactVersion,
		SchemaSHA256: contract.SHA256(),
		Schema:       contract.Schema(),
	}, nil
}

// contract returns the schema contract of core. A line without its own
// schema command has the reviewed schema; otherwise the core's schema is a
// function of its binary, so it is kept by the archive's digest once nodeID
// sent it.
func (application *Service) contract(ctx context.Context, nodeID string, core store.CachedArtifact) (*schema.Contract, error) {
	line, found := singbox.LineOf(core.ExactVersion)
	if !found {
		return nil, schemaUnavailable(core.ExactVersion)
	}
	if !line.NativeSchema {
		return schema.Reviewed113()
	}
	if contract, found := application.nativeSchemas.get(core.SHA256); found {
		return contract, nil
	}
	raw, err := application.coreSchema(ctx, nodeID, core.ID)
	if err != nil {
		return nil, err
	}
	contract, err := schema.Prepare(raw)
	if err != nil {
		return nil, fmt.Errorf("prepare the configuration schema of sing-box %s: %w", core.ExactVersion, err)
	}
	application.nativeSchemas.put(core.SHA256, contract)
	return contract, nil
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

// ValidateForCore checks a revision against the configuration schema of the
// cached core coreArtifactID on nodeID before the revision reaches the node.
// A revision is accepted when no schema can be had for the core; the node's
// sing-box check still gates it.
func (application *Service) ValidateForCore(ctx context.Context, nodeID string, revision store.CanonicalRevision, coreArtifactID string) error {
	core, err := application.cores.CachedArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return err
	}
	contract, err := application.contract(ctx, nodeID, core)
	if err != nil {
		return nil
	}
	if err := contract.Validate(revision.Document); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigurationSchemaValidation, err)
	}
	return nil
}

// ErrConfigurationSchemaValidation rejects a configuration that the core's
// schema does not accept.
var ErrConfigurationSchemaValidation = errors.New("configuration schema validation failed")
