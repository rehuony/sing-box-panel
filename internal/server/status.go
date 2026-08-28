// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/httpapi"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type statusProvider struct {
	database *store.Store
	build    buildinfo.Info
	commands *application.Application
	runtime  *runtimeServices
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
		live := provider.runtime.manager.ObserveLiveIdentity()
		status.Running = live.Running
		if live.Running {
			status.RunningVersion = stringPointer(live.ExactVersion.String())
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
	if coreArtifactID != "" && provider.commands != nil {
		support, supportErr := provider.commands.ConfigurationSupport(ctx, coreArtifactID)
		if supportErr != nil {
			return httpapi.SystemStatus{}, supportErr
		}
		status.ConfigurationState = "unsupported"
		if support.Supported {
			status.ConfigurationState = support.AdapterID + "@" + support.Revision
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
		Adapter: httpapi.DashboardAdapter{
			Supported: false,
			Label:     "No core selected",
			Warning:   &warning,
		},
	}
	if provider.runtime != nil {
		live := provider.runtime.manager.ObserveLiveIdentity()
		if live.Running {
			result.View.ExactVersion = live.ExactVersion.String()
			result.Running = &httpapi.DashboardRuntime{
				ExactVersion: live.ExactVersion.String(),
				ArtifactName: live.ArtifactID,
				Digest:       live.ArtifactDigest.String(),
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
	if result.Applied != nil && provider.commands != nil {
		bundle, err := provider.database.GetActivationBundle(ctx, bootstrap.Hub.AppliedBundleID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		startup, err := provider.database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		support, err := provider.commands.ConfigurationSupport(ctx, startup.CoreArtifactID)
		if err != nil {
			return httpapi.DashboardContext{}, err
		}
		result.Adapter.Supported = support.Supported
		if support.Supported {
			result.Adapter.Label = support.AdapterID + "@" + support.Revision
			result.Adapter.Warning = nil
		} else {
			result.Adapter.Label = "Unsupported core profile"
			result.Adapter.Warning = stringPointer(support.Reason)
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
