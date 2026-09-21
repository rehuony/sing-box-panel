// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/singbox"
	"github.com/rehuony/sing-box-panel/internal/store"
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
	CanonicalRevision CanonicalSnapshot    `json:"canonical_revision"`
	CoreArtifact      CoreArtifact         `json:"core_artifact"`
	Support           ConfigurationSupport `json:"support"`
	Config            json.RawMessage      `json:"config"`
}

func (application *Application) configurationSupport(core store.CoreArtifact) ConfigurationSupport {
	_, err := singbox.ConfigurationSchema(core.ExactVersion)
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

// ConfigurationSupport reports whether the exact sing-box version has a native
// browser schema. Raw JSON editing, checking, and execution do not depend on
// this presentation capability.
func (application *Application) ConfigurationSupport(
	ctx context.Context,
	coreArtifactID string,
) (ConfigurationSupport, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return ConfigurationSupport{}, err
	}
	return application.configurationSupport(core), nil
}

// ConfigurationSchema returns the canonical browser schema selected only
// by the artifact's exact sing-box version.
func (application *Application) ConfigurationSchema(
	ctx context.Context,
	coreArtifactID string,
) (ConfigurationSchema, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return ConfigurationSchema{}, err
	}
	contract, err := singbox.ConfigurationSchema(core.ExactVersion)
	if err != nil {
		return ConfigurationSchema{}, err
	}
	return ConfigurationSchema{
		ExactVersion: core.ExactVersion,
		SchemaSHA256: contract.SchemaSHA256,
		Schema:       append(json.RawMessage(nil), contract.Schema...),
	}, nil
}

// PreviewConfiguration snapshots the current valid file for the selected core
// without persisting startup bytes.
func (application *Application) PreviewConfiguration(
	ctx context.Context,
	request ConfigurationPreviewRequest,
) (ConfigurationPreview, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(request.CoreArtifactID))
	if err != nil {
		return ConfigurationPreview{}, err
	}
	file, err := application.database.ConfigurationFile(ctx)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	if file.Revision == 0 {
		return ConfigurationPreview{}, errors.New("no configuration has been saved")
	}
	if file.CanonicalRevisionID == "" {
		return ConfigurationPreview{}, store.ErrConfigurationFileUnparsed
	}
	revision, err := application.database.GetCanonicalRevision(ctx, file.CanonicalRevisionID)
	if err != nil {
		return ConfigurationPreview{}, err
	}

	document, err := configuration.Parse(revision.Document)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	return ConfigurationPreview{
		CanonicalRevision: snapshot(revision),
		CoreArtifact:      coreArtifact(core),
		Support:           application.configurationSupport(core),
		Config:            append(json.RawMessage(nil), document.CanonicalJSON()...),
	}, nil
}
