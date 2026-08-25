// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var (
	ErrStructuredCapabilityUnavailable = errors.New("structured capability is unavailable")
	ErrCompatibleCapabilityNotAccepted = errors.New("compatible capability requires explicit acceptance")
	ErrUnsupportedActiveFact           = errors.New("active canonical fact is unsupported by the exact core version")
)

type StructuredRenderRequest struct {
	CoreVersion     string
	CoreArtifactID  string
	AllowCompatible bool
}

type StructuredArtifact struct {
	ID                  string                     `json:"id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	CapabilityCommit    string                     `json:"capability_commit"`
	CapabilityDigest    string                     `json:"capability_digest"`
	RendererVersion     string                     `json:"renderer_version"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	Config              json.RawMessage            `json:"config"`
	Diagnostics         []capability.Diagnostic    `json:"diagnostics"`
	State               store.StartupArtifactState `json:"state"`
}

type StructuredRender struct {
	Resolution CoreVersionResolution `json:"resolution"`
	Pin        CapabilityPinView     `json:"pin"`
	Artifact   StructuredArtifact    `json:"artifact"`
	Task       Task                  `json:"task"`
}

// RenderStructured projects the current canonical head through the exact,
// locally pinned declarative manifest and atomically queues a real binary
// check. It never chooses latest and never executes manifest-provided code.
func (application *Application) RenderStructured(
	ctx context.Context,
	request StructuredRenderRequest,
) (StructuredRender, error) {
	resolution, err := application.ResolveCoreVersion(ctx, request.CoreVersion)
	if err != nil {
		return StructuredRender{}, err
	}
	core, err := application.resolveCoreArtifact(ctx, resolution, strings.TrimSpace(request.CoreArtifactID))
	if err != nil {
		return StructuredRender{}, err
	}
	manifest, pin, err := application.PinnedCapabilityManifest(ctx, resolution.ExactVersion)
	if err != nil {
		if errors.Is(err, store.ErrCapabilityPinNotFound) || errors.Is(err, ErrCapabilityCandidateQuarantined) {
			return StructuredRender{}, fmt.Errorf("%w: %v", ErrStructuredCapabilityUnavailable, err)
		}
		return StructuredRender{}, err
	}
	switch manifest.SupportLevel() {
	case capability.SupportNativeStructured:
	case capability.SupportCompatibleStructured:
		if !request.AllowCompatible {
			return StructuredRender{}, ErrCompatibleCapabilityNotAccepted
		}
	default:
		return StructuredRender{}, fmt.Errorf(
			"%w: exact version %s is %s",
			ErrStructuredCapabilityUnavailable,
			resolution.ExactVersion,
			manifest.SupportLevel(),
		)
	}

	head, err := application.database.Head(ctx)
	if err != nil {
		return StructuredRender{}, err
	}
	if head == nil {
		return StructuredRender{}, errors.New("no canonical revision has been saved")
	}
	document, err := canonical.Parse(head.Document)
	if err != nil {
		return StructuredRender{}, err
	}
	projector, err := capability.NewProjector(manifest)
	if err != nil {
		return StructuredRender{}, err
	}
	projection, err := projector.Project(document.Map())
	if err != nil {
		return StructuredRender{}, err
	}
	for _, diagnostic := range projection.Diagnostics {
		if diagnostic.Code == "fact_omitted" {
			return StructuredRender{}, fmt.Errorf("%w: %s", ErrUnsupportedActiveFact, diagnostic.FactID)
		}
	}
	config, err := json.Marshal(projection.Document)
	if err != nil {
		return StructuredRender{}, fmt.Errorf("encode projected configuration: %w", err)
	}
	diagnostics, err := json.Marshal(projection.Diagnostics)
	if err != nil {
		return StructuredRender{}, fmt.Errorf("encode projection diagnostics: %w", err)
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return StructuredRender{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return StructuredRender{}, err
	}
	createdAt := application.now().UTC()
	payload, err := json.Marshal(map[string]string{"startup_artifact_id": startupID})
	if err != nil {
		return StructuredRender{}, err
	}
	stored, err := application.database.CreateStartupArtifactAndCheckTask(ctx, store.StartupArtifact{
		ID: startupID, Kind: store.StartupArtifactStructured,
		CanonicalRevisionID: head.ID, ExactCoreVersion: resolution.ExactVersion,
		CapabilityCommit: pin.CommitSHA, CapabilityDigest: pin.ManifestSHA256,
		RendererVersion: "capability-projector-v1", CoreArtifactID: core.ID,
		ConfigBytes: config, Diagnostics: diagnostics, CreatedAt: createdAt,
	}, store.NewTask{
		ID: taskID, IdempotencyKey: "startup-check:" + startupID,
		Lane: store.TaskLaneMaintenance, Kind: "startup-check", Payload: payload, CreatedAt: createdAt,
	}, store.StructuredStartupEvidence{
		ExpectedCanonicalHeadID: head.ID,
		CapabilityRepository:    pin.Repository,
		CapabilityCommit:        pin.CommitSHA,
		CapabilityDigest:        pin.ManifestSHA256,
		CapabilitySupport:       manifest.SupportLevel(),
	})
	if err != nil {
		if errors.Is(err, store.ErrStructuredStartupEvidenceStale) {
			return StructuredRender{}, fmt.Errorf("%w: %v", ErrStructuredCapabilityUnavailable, err)
		}
		return StructuredRender{}, err
	}
	return StructuredRender{
		Resolution: resolution,
		Pin:        pin,
		Artifact: StructuredArtifact{
			ID: stored.Artifact.ID, CanonicalRevisionID: stored.Artifact.CanonicalRevisionID,
			ExactCoreVersion: stored.Artifact.ExactCoreVersion,
			CapabilityCommit: stored.Artifact.CapabilityCommit,
			CapabilityDigest: stored.Artifact.CapabilityDigest,
			RendererVersion:  stored.Artifact.RendererVersion,
			CoreArtifactID:   stored.Artifact.CoreArtifactID,
			ConfigSHA256:     stored.Artifact.ConfigSHA256,
			Config:           append(json.RawMessage(nil), stored.Artifact.ConfigBytes...),
			Diagnostics:      append([]capability.Diagnostic(nil), projection.Diagnostics...),
			State:            stored.Artifact.State,
		},
		Task: applicationTask(stored.Task),
	}, nil
}

func IsStructuredCapabilityUnavailable(err error) bool {
	return errors.Is(err, ErrStructuredCapabilityUnavailable)
}

func IsCompatibleCapabilityNotAccepted(err error) bool {
	return errors.Is(err, ErrCompatibleCapabilityNotAccepted)
}

func IsUnsupportedActiveFact(err error) bool {
	return errors.Is(err, ErrUnsupportedActiveFact)
}
