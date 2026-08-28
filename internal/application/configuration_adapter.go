// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
	singbox11115 "github.com/rehuony/sing-box-panel/internal/configuration/adapter/singbox/v1_11_15"
	singbox11225 "github.com/rehuony/sing-box-panel/internal/configuration/adapter/singbox/v1_12_25"
	singbox11319 "github.com/rehuony/sing-box-panel/internal/configuration/adapter/singbox/v1_13_19"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var compiledConfigurationRegistry = adapter.MustNewRegistry(
	singbox11115.New(),
	singbox11225.New(),
	singbox11319.New(),
)

type ConfigurationAdapterSupport struct {
	Supported  bool               `json:"supported"`
	Profile    adapter.Profile    `json:"profile"`
	AdapterID  string             `json:"adapter_id,omitempty"`
	Revision   string             `json:"adapter_revision,omitempty"`
	Provenance adapter.Provenance `json:"provenance,omitempty"`
	Reason     string             `json:"reason,omitempty"`
}

type ConfigurationPreviewRequest struct {
	CoreArtifactID      string `json:"core_artifact_id"`
	CanonicalRevisionID string `json:"canonical_revision_id,omitempty"`
}

type ConfigurationPreview struct {
	CanonicalRevision CanonicalSnapshot           `json:"canonical_revision"`
	CoreArtifact      CoreArtifact                `json:"core_artifact"`
	Support           ConfigurationAdapterSupport `json:"support"`
	Config            json.RawMessage             `json:"config"`
	Diagnostics       []adapter.Diagnostic        `json:"diagnostics"`
	IgnoredDigest     string                      `json:"ignored_digest,omitempty"`
}

func coreArtifactProfile(core store.CoreArtifact) adapter.Profile {
	return adapter.Profile{
		ExactVersion:       core.ExactVersion,
		OperatingSystem:    core.OperatingSystem,
		Architecture:       core.Architecture,
		Variant:            core.Variant,
		FeatureFingerprint: append(json.RawMessage(nil), core.FeatureFingerprint...),
	}
}

func (application *Application) configurationSupport(core store.CoreArtifact) ConfigurationAdapterSupport {
	profile := coreArtifactProfile(core)
	resolved, err := application.configurationAdapters.Resolve(profile)
	if err != nil {
		return ConfigurationAdapterSupport{
			Supported: false,
			Profile:   profile,
			Reason:    err.Error(),
		}
	}
	return ConfigurationAdapterSupport{
		Supported:  true,
		Profile:    profile,
		AdapterID:  resolved.ID(),
		Revision:   resolved.Revision(),
		Provenance: resolved.Provenance(),
	}
}

// ConfigurationSupport reports whether one installed binary matches a reviewed,
// compiled adapter. Installation and inspection remain available even when it
// does not; projection and execution fail closed.
func (application *Application) ConfigurationSupport(
	ctx context.Context,
	coreArtifactID string,
) (ConfigurationAdapterSupport, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(coreArtifactID))
	if err != nil {
		return ConfigurationAdapterSupport{}, err
	}
	return application.configurationSupport(core), nil
}

// PreviewConfiguration projects one immutable canonical revision without
// persisting startup bytes. An empty revision ID means the current global head.
func (application *Application) PreviewConfiguration(
	ctx context.Context,
	request ConfigurationPreviewRequest,
) (ConfigurationPreview, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(request.CoreArtifactID))
	if err != nil {
		return ConfigurationPreview{}, err
	}
	if core.VerificationState != store.CoreArtifactVerified {
		return ConfigurationPreview{}, fmt.Errorf("%w: %s is %s", ErrCoreArtifactVerificationBlocked, core.ID, core.VerificationState)
	}

	var revision store.CanonicalRevision
	if strings.TrimSpace(request.CanonicalRevisionID) == "" {
		head, headErr := application.database.Head(ctx)
		if headErr != nil {
			return ConfigurationPreview{}, headErr
		}
		if head == nil {
			return ConfigurationPreview{}, errors.New("no canonical revision has been saved")
		}
		revision = *head
	} else {
		revision, err = application.database.GetCanonicalRevision(ctx, strings.TrimSpace(request.CanonicalRevisionID))
		if err != nil {
			return ConfigurationPreview{}, err
		}
	}

	support := application.configurationSupport(core)
	if !support.Supported {
		return ConfigurationPreview{}, fmt.Errorf("%w: %s", adapter.ErrUnsupportedCoreProfile, support.Reason)
	}
	result, err := application.configurationAdapters.Project(support.Profile, adapter.Request{
		CanonicalJSON: revision.Document,
	})
	if err != nil {
		return ConfigurationPreview{}, err
	}
	diagnostics := make([]adapter.Diagnostic, len(result.Diagnostics))
	copy(diagnostics, result.Diagnostics)
	return ConfigurationPreview{
		CanonicalRevision: snapshot(revision),
		CoreArtifact:      coreArtifact(core),
		Support:           support,
		Config:            append(json.RawMessage(nil), result.ConfigJSON...),
		Diagnostics:       diagnostics,
		IgnoredDigest:     result.IgnoredDigest,
	}, nil
}
