// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/httpapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/localnode"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

type statusProvider struct {
	database *store.Store
	build    buildinfo.Info
	configs  *nodeconfig.Service
	runtime  *localnode.Runtime
}

func (provider *statusProvider) SystemStatus(ctx context.Context) (httpapi.SystemStatus, error) {
	bootstrap, err := provider.database.Bootstrap(ctx)
	if err != nil {
		return httpapi.SystemStatus{}, err
	}
	status := httpapi.SystemStatus{
		PanelVersion:       provider.build.Version,
		AppliedBundleID:    stringPointer(bootstrap.Hub.AppliedBundleID),
		Running:            false,
		ConfigurationState: "unresolved",
	}
	coreArtifactID := ""
	if provider.runtime != nil {
		live := provider.runtime.LiveCore()
		status.Running = live.Running
		if live.Running {
			status.RunningVersion = stringPointer(live.ExactVersion)
			status.RunningArtifact = stringPointer(live.ArtifactID)
			coreArtifactID = live.ArtifactID
		}
	}
	if coreArtifactID == "" && bootstrap.Hub.AppliedBundleID != "" {
		bundle, bundleErr := provider.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
		if bundleErr != nil {
			return httpapi.SystemStatus{}, bundleErr
		}
		startup, startupErr := provider.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
		if startupErr != nil {
			return httpapi.SystemStatus{}, startupErr
		}
		coreArtifactID = startup.CoreArtifactID
	}
	if coreArtifactID != "" && provider.configs != nil {
		support, supportErr := provider.configs.ConfigurationSupport(ctx, coreArtifactID)
		if supportErr != nil {
			return httpapi.SystemStatus{}, supportErr
		}
		status.ConfigurationState = "unsupported"
		if support.Structured {
			status.ConfigurationState = "schema@" + support.ExactVersion
		} else {
			status.ConfigurationState = "raw"
		}
	}
	if bootstrap.Head != nil {
		status.CanonicalRevision = bootstrap.Head.Sequence
	}
	return status, nil
}

func (provider *statusProvider) DashboardContext(ctx context.Context) (httpapi.DashboardContext, error) {
	bootstrap, err := provider.database.Bootstrap(ctx)
	if err != nil {
		return httpapi.DashboardContext{}, err
	}
	warning := "Install or import an exact sing-box artifact before preparing an activation bundle."
	result := httpapi.DashboardContext{
		View: httpapi.DashboardView{ExactVersion: "Not selected"},
		Canonical: httpapi.DashboardCanonical{
			Revision:            0,
			SavedAt:             bootstrap.Hub.UpdatedAt,
			HasUnappliedChanges: false,
		},
		Configuration: httpapi.DashboardConfiguration{
			Supported: false,
			Label:     "No core selected",
			Warning:   &warning,
		},
	}
	if provider.runtime != nil {
		live := provider.runtime.LiveCore()
		if live.Running {
			result.View.ExactVersion = live.ExactVersion
			result.Running = &httpapi.DashboardRuntime{
				ExactVersion: live.ExactVersion,
				ArtifactName: live.ArtifactID,
				Digest:       live.ArtifactDigest,
			}
		}
	}
	if bootstrap.Head != nil {
		result.Canonical.Revision = bootstrap.Head.Sequence
		result.Canonical.SavedAt = bootstrap.Head.CreatedAt
		result.Canonical.HasUnappliedChanges = bootstrap.Hub.AppliedBundleID == ""
	}
	if bootstrap.Hub.AppliedBundleID != "" {
		bundle, err := provider.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		startup, err := provider.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		revision, err := provider.database.GetCanonicalRevision(ctx, startup.CanonicalRevisionID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		appliedAt := bundle.CreatedAt
		if bootstrap.Hub.AppliedAt != nil {
			appliedAt = *bootstrap.Hub.AppliedAt
		}
		result.Applied = &httpapi.DashboardApplied{
			Bundle: bundle.ID, Revision: revision.Sequence, AppliedAt: appliedAt,
		}
		if result.View.ExactVersion == "Not selected" {
			result.View.ExactVersion = startup.ExactCoreVersion
		}
		result.Canonical.HasUnappliedChanges = bootstrap.Head != nil && bootstrap.Head.ID != startup.CanonicalRevisionID
	}
	if result.Applied != nil && provider.configs != nil {
		bundle, err := provider.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		startup, err := provider.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		support, err := provider.configs.ConfigurationSupport(ctx, startup.CoreArtifactID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		result.Configuration.Supported = support.Structured
		if support.Structured {
			result.Configuration.Label = "Schema " + support.ExactVersion
			result.Configuration.Warning = nil
		} else {
			result.Configuration.Label = "Raw JSON"
			warning := "Structured editing is unavailable for this version; exact-binary check still gates activation."
			result.Configuration.Warning = &warning
		}
	}
	return result, nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
