// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/manualjson"
	"github.com/rehuony/sing-box-panel/internal/reconcile"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var (
	ErrManualReattachUnavailable  = errors.New("manual reattach is unavailable")
	ErrManualReattachPreviewStale = errors.New("manual reattach preview is stale")
)

type ManualReattachPinEvidence struct {
	Repository     string                  `json:"repository"`
	CommitSHA      string                  `json:"commit_sha"`
	ManifestSHA256 string                  `json:"manifest_sha256"`
	SupportLevel   capability.SupportLevel `json:"support_level"`
}

// ManualReattachEvidence is deliberately self-contained. It is safe to carry
// through an interactive preview because it contains identities and digests,
// never manual configuration bytes or credentials.
type ManualReattachEvidence struct {
	StartupArtifactID  string                    `json:"startup_artifact_id"`
	ConfigSHA256       string                    `json:"config_sha256"`
	BaseRevisionID     string                    `json:"base_revision_id"`
	BaseRevisionSHA256 string                    `json:"base_revision_sha256"`
	CurrentHeadID      string                    `json:"current_head_id"`
	CurrentHeadSHA256  string                    `json:"current_head_sha256"`
	ExactCoreVersion   string                    `json:"exact_core_version"`
	CoreArtifactID     string                    `json:"core_artifact_id"`
	Capability         ManualReattachPinEvidence `json:"capability"`
}

type ManualReattachPreview struct {
	Evidence      ManualReattachEvidence `json:"evidence"`
	Base          CanonicalSnapshot      `json:"base"`
	Current       CanonicalSnapshot      `json:"current"`
	Manual        json.RawMessage        `json:"manual"`
	OwnedPartial  json.RawMessage        `json:"owned_partial"`
	Merged        json.RawMessage        `json:"merged"`
	ResidualPaths []string               `json:"residual_paths"`
	Conflicts     []reconcile.Conflict   `json:"conflicts"`
}

type ManualReattachApplyRequest struct {
	StartupArtifactID string                      `json:"-"`
	Evidence          ManualReattachEvidence      `json:"evidence"`
	Decisions         map[string]reconcile.Choice `json:"decisions"`
}

type ManualReattachSave struct {
	Preview  ManualReattachPreview `json:"preview"`
	Revision CanonicalSnapshot     `json:"revision"`
	Artifact ManualArtifact        `json:"artifact"`
	Task     Task                  `json:"task"`
}

// PreviewManualReattach is read-only. It reverses only manifest-owned paths,
// overlays that partial value onto the manual artifact's immutable base, then
// performs a deterministic base/current/manual comparison.
func (application *Application) PreviewManualReattach(
	ctx context.Context,
	startupArtifactID string,
) (ManualReattachPreview, error) {
	artifact, err := application.database.GetStartupArtifact(ctx, startupArtifactID)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	if artifact.Kind != store.StartupArtifactManual {
		return ManualReattachPreview{}, errors.New("startup artifact is not manual JSON")
	}
	base, err := application.database.GetCanonicalRevision(ctx, artifact.CanonicalRevisionID)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	head, err := application.database.Head(ctx)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	if head == nil {
		return ManualReattachPreview{}, fmt.Errorf("%w: canonical head is not initialized", ErrManualReattachUnavailable)
	}
	manifest, pin, err := application.PinnedCapabilityManifest(ctx, artifact.ExactCoreVersion)
	if err != nil {
		return ManualReattachPreview{}, fmt.Errorf(
			"%w: exact version %s has no usable pinned structured manifest: %v",
			ErrManualReattachUnavailable,
			artifact.ExactCoreVersion,
			err,
		)
	}
	if manifest.CoreVersion().String() != artifact.ExactCoreVersion ||
		!structuredSupportLevel(manifest.SupportLevel()) {
		return ManualReattachPreview{}, fmt.Errorf(
			"%w: pinned manifest does not provide structured support for exact version %s",
			ErrManualReattachUnavailable,
			artifact.ExactCoreVersion,
		)
	}
	projector, err := capability.NewProjector(manifest)
	if err != nil {
		return ManualReattachPreview{}, fmt.Errorf("%w: %v", ErrManualReattachUnavailable, err)
	}
	core, err := application.database.GetCoreArtifact(ctx, artifact.CoreArtifactID)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	if core.ExactVersion != artifact.ExactCoreVersion ||
		core.ReportedVersion != artifact.ExactCoreVersion {
		return ManualReattachPreview{}, fmt.Errorf(
			"%w: manual artifact and core binary exact versions do not match",
			ErrManualReattachUnavailable,
		)
	}
	version, err := coreartifact.ParseExactVersion(artifact.ExactCoreVersion)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	binaryDigest, err := coreartifact.ParseSHA256(core.BinarySHA256)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	manualDocument, err := manualjson.Parse(artifact.ConfigBytes, manualjson.Binding{
		CoreVersion: version, ArtifactDigest: binaryDigest, BaseRevisionID: base.ID,
	})
	if err != nil {
		return ManualReattachPreview{}, err
	}
	partial, err := projector.ReversePartial(manualDocument.Object())
	if err != nil {
		return ManualReattachPreview{}, fmt.Errorf("reverse manual owned paths: %w", err)
	}

	baseDocument, err := canonical.Parse(base.Document)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	currentDocument, err := canonical.Parse(head.Document)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	manualMap, err := overlayCanonical(baseDocument.Map(), partial.Canonical)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	preview, err := reconcile.ThreeWay(baseDocument.Map(), currentDocument.Map(), manualMap)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	manualJSON, err := canonicalMapJSON(manualMap)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	partialJSON, err := marshalObject(partial.Canonical)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	mergedJSON, err := canonicalMapJSON(preview.Merged)
	if err != nil {
		return ManualReattachPreview{}, err
	}
	residual := append([]string(nil), partial.ResidualPaths...)
	sort.Strings(residual)

	return ManualReattachPreview{
		Evidence: ManualReattachEvidence{
			StartupArtifactID: artifact.ID, ConfigSHA256: artifact.ConfigSHA256,
			BaseRevisionID: base.ID, BaseRevisionSHA256: base.SHA256,
			CurrentHeadID: head.ID, CurrentHeadSHA256: head.SHA256,
			ExactCoreVersion: artifact.ExactCoreVersion, CoreArtifactID: artifact.CoreArtifactID,
			Capability: ManualReattachPinEvidence{
				Repository: pin.Repository, CommitSHA: pin.CommitSHA,
				ManifestSHA256: pin.ManifestSHA256, SupportLevel: pin.SupportLevel,
			},
		},
		Base: snapshot(base), Current: snapshot(*head), Manual: manualJSON,
		OwnedPartial: partialJSON, Merged: mergedJSON, ResidualPaths: residual,
		Conflicts: append([]reconcile.Conflict(nil), preview.Conflicts...),
	}, nil
}

// ApplyManualReattach requires evidence from a previous preview and an exact
// decision for every conflict. It recomputes the preview before resolving, and
// the store validates the same evidence again inside the commit transaction.
func (application *Application) ApplyManualReattach(
	ctx context.Context,
	request ManualReattachApplyRequest,
) (ManualReattachSave, error) {
	if request.StartupArtifactID == "" || request.Evidence.StartupArtifactID == "" ||
		request.StartupArtifactID != request.Evidence.StartupArtifactID {
		return ManualReattachSave{}, errors.New("manual reattach artifact id does not match preview evidence")
	}
	preview, err := application.PreviewManualReattach(ctx, request.StartupArtifactID)
	if err != nil {
		return ManualReattachSave{}, err
	}
	if preview.Evidence != request.Evidence {
		return ManualReattachSave{}, fmt.Errorf(
			"%w: current database identity no longer matches supplied evidence",
			ErrManualReattachPreviewStale,
		)
	}
	base, err := decodeCanonicalMap(preview.Base.Document)
	if err != nil {
		return ManualReattachSave{}, err
	}
	current, err := decodeCanonicalMap(preview.Current.Document)
	if err != nil {
		return ManualReattachSave{}, err
	}
	manual, err := decodeCanonicalMap(preview.Manual)
	if err != nil {
		return ManualReattachSave{}, err
	}
	resolved, err := reconcile.Resolve(base, current, manual, request.Decisions)
	if err != nil {
		return ManualReattachSave{}, err
	}
	resolvedJSON, err := canonicalMapJSON(resolved)
	if err != nil {
		return ManualReattachSave{}, err
	}

	source, err := application.database.GetStartupArtifact(ctx, request.StartupArtifactID)
	if err != nil {
		return ManualReattachSave{}, err
	}
	revisionID, err := application.newID("rev")
	if err != nil {
		return ManualReattachSave{}, err
	}
	commandID, err := application.newID("cmd")
	if err != nil {
		return ManualReattachSave{}, err
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return ManualReattachSave{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return ManualReattachSave{}, err
	}
	createdAt := application.now().UTC()
	diagnostics, err := json.Marshal([]map[string]any{{
		"code": "manual_json_reattached", "source_startup_artifact_id": source.ID,
		"capability_repository": preview.Evidence.Capability.Repository,
		"capability_commit":     preview.Evidence.Capability.CommitSHA,
		"manifest_sha256":       preview.Evidence.Capability.ManifestSHA256,
		"residual_owner":        "manual_artifact", "residual_paths": preview.ResidualPaths,
		"conflict_decisions": request.Decisions,
	}})
	if err != nil {
		return ManualReattachSave{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"startup_artifact_id": startupID,
		"reattach_evidence":   request.Evidence,
		"conflict_decisions":  request.Decisions,
		"residual_paths":      preview.ResidualPaths,
	})
	if err != nil {
		return ManualReattachSave{}, err
	}
	stored, err := application.database.SaveReattachedManualArtifactAndTask(
		ctx,
		store.SaveReattachedManualArtifactInput{
			Evidence: store.ManualReattachEvidence{
				SourceArtifactID:     request.Evidence.StartupArtifactID,
				SourceConfigSHA256:   request.Evidence.ConfigSHA256,
				BaseRevisionID:       request.Evidence.BaseRevisionID,
				BaseRevisionSHA256:   request.Evidence.BaseRevisionSHA256,
				ExpectedHead:         request.Evidence.CurrentHeadID,
				ExpectedHeadSHA256:   request.Evidence.CurrentHeadSHA256,
				ExactCoreVersion:     request.Evidence.ExactCoreVersion,
				CoreArtifactID:       request.Evidence.CoreArtifactID,
				CapabilityRepository: request.Evidence.Capability.Repository,
				CapabilityCommit:     request.Evidence.Capability.CommitSHA,
				CapabilitySHA256:     request.Evidence.Capability.ManifestSHA256,
				CapabilitySupport:    request.Evidence.Capability.SupportLevel,
			},
			Revision: store.NewCanonicalRevision{
				ID: revisionID, SchemaVersion: canonical.SchemaVersion,
				Document: resolvedJSON, CommandID: commandID, CreatedAt: createdAt,
			},
			Artifact: store.NewManualStartupArtifact{
				ID: startupID, ExactCoreVersion: source.ExactCoreVersion,
				RendererVersion: "manual-json-reattach-v1", CoreArtifactID: source.CoreArtifactID,
				ConfigBytes: source.ConfigBytes, ConfigSHA256: source.ConfigSHA256,
				Diagnostics: diagnostics, CreatedAt: createdAt,
			},
			CheckTask: store.NewTask{
				ID: taskID, IdempotencyKey: "startup-check:" + startupID,
				Lane: store.TaskLaneMaintenance, Kind: "startup-check",
				Payload: payload, CreatedAt: createdAt,
			},
		},
	)
	if err != nil {
		if errors.Is(err, store.ErrManualReattachEvidenceStale) ||
			errors.Is(err, store.ErrRevisionConflict) {
			return ManualReattachSave{}, fmt.Errorf("%w: %v", ErrManualReattachPreviewStale, err)
		}
		if errors.Is(err, store.ErrCapabilityManifestQuarantined) {
			return ManualReattachSave{}, fmt.Errorf("%w: %v", ErrManualReattachUnavailable, err)
		}
		return ManualReattachSave{}, err
	}
	return ManualReattachSave{
		Preview: preview, Revision: snapshot(stored.Revision),
		Artifact: manualArtifact(stored.StartupArtifact), Task: applicationTask(stored.CheckTask),
	}, nil
}

func overlayCanonical(base, partial map[string]any) (map[string]any, error) {
	if base == nil || partial == nil {
		return nil, errors.New("manual reattach overlay roots must be objects")
	}
	result, err := cloneObject(base)
	if err != nil {
		return nil, err
	}
	if err := overlayObject(result, partial); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	document, err := canonical.Parse(encoded)
	if err != nil {
		return nil, fmt.Errorf("reverse projection produced invalid canonical partial overlay: %w", err)
	}
	return document.Map(), nil
}

func overlayObject(destination, source map[string]any) error {
	for key, sourceValue := range source {
		sourceObject, sourceIsObject := sourceValue.(map[string]any)
		destinationObject, destinationIsObject := destination[key].(map[string]any)
		if sourceIsObject && destinationIsObject {
			if err := overlayObject(destinationObject, sourceObject); err != nil {
				return err
			}
			continue
		}
		clone, err := cloneValue(sourceValue)
		if err != nil {
			return err
		}
		destination[key] = clone
	}
	return nil
}

func cloneObject(value map[string]any) (map[string]any, error) {
	clone, err := cloneValue(value)
	if err != nil {
		return nil, err
	}
	result, ok := clone.(map[string]any)
	if !ok {
		return nil, errors.New("cloned JSON root is not an object")
	}
	return result, nil
}

func cloneValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func canonicalMapJSON(value map[string]any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	document, err := canonical.Parse(encoded)
	if err != nil {
		return nil, err
	}
	return document.CanonicalJSON(), nil
}

func marshalObject(value map[string]any) (json.RawMessage, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func decodeCanonicalMap(data []byte) (map[string]any, error) {
	document, err := canonical.Parse(data)
	if err != nil {
		return nil, err
	}
	return document.Map(), nil
}

func structuredSupportLevel(level capability.SupportLevel) bool {
	return level == capability.SupportNativeStructured ||
		level == capability.SupportCompatibleStructured
}

func IsManualReattachUnavailable(err error) bool {
	return errors.Is(err, ErrManualReattachUnavailable)
}

func IsManualReattachPreviewStale(err error) bool {
	return errors.Is(err, ErrManualReattachPreviewStale) ||
		errors.Is(err, store.ErrManualReattachEvidenceStale)
}

func IsManualReattachUnresolved(err error) bool {
	return errors.Is(err, reconcile.ErrUnresolvedConflict)
}
