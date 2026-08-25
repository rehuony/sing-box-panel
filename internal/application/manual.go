// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/manualjson"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrManualDetachSource = errors.New("manual detach source is invalid")

type ManualReplaceRequest struct {
	ExpectedHead    string
	CoreVersion     string
	CoreArtifactID  string
	Raw             []byte
	AllowCompatible bool
}

type ManualArtifact struct {
	ID                  string                     `json:"id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	Raw                 string                     `json:"raw"`
	Diagnostics         json.RawMessage            `json:"diagnostics"`
	State               store.StartupArtifactState `json:"state"`
	CheckedAt           *time.Time                 `json:"checked_at,omitempty"`
	CreatedAt           time.Time                  `json:"created_at"`
}

type ManualSave struct {
	Resolution CoreVersionResolution `json:"resolution"`
	Preview    ManualReplacePreview  `json:"preview"`
	Revision   CanonicalSnapshot     `json:"revision"`
	NoChange   bool                  `json:"no_change"`
	Artifact   ManualArtifact        `json:"artifact"`
	Task       Task                  `json:"task"`
}

// ManualReplacePreview is a read-only explanation of the exact save that
// ReplaceManualJSON will attempt. It never returns raw configuration bytes.
// ProposedCanonical contains only the immutable canonical base plus fields
// proven reversible by the exact pinned capability.
type ManualReplacePreview struct {
	Resolution     CoreVersionResolution `json:"resolution"`
	Base           CanonicalSnapshot     `json:"base"`
	CoreArtifactID string                `json:"core_artifact_id"`
	ConfigSHA256   string                `json:"config_sha256"`
	Reverse        ManualReverseMapping  `json:"reverse"`
}

type ManualReverseMapping struct {
	Available         bool                       `json:"available"`
	ReasonCode        string                     `json:"reason_code,omitempty"`
	Capability        *ManualReattachPinEvidence `json:"capability,omitempty"`
	OwnedPartial      json.RawMessage            `json:"owned_partial"`
	ProposedCanonical json.RawMessage            `json:"proposed_canonical"`
	ResidualPaths     []string                   `json:"residual_paths"`
	CanonicalChanged  bool                       `json:"canonical_changed"`
}

type preparedManualReplace struct {
	request           ManualReplaceRequest
	core              store.CoreArtifact
	document          *manualjson.Document
	canonicalDocument json.RawMessage
	preview           ManualReplacePreview
}

// DetachManualJSON copies one structured candidate's exact projected bytes
// into a new manual candidate bound to the current canonical head, exact
// version, and binary. The source remains immutable; the new candidate gets
// its own real startup check and no longer depends on a capability pin.
func (application *Application) DetachManualJSON(
	ctx context.Context,
	startupArtifactID string,
) (ManualSave, error) {
	source, err := application.database.GetStartupArtifact(ctx, strings.TrimSpace(startupArtifactID))
	if err != nil {
		return ManualSave{}, err
	}
	if source.Kind != store.StartupArtifactStructured {
		return ManualSave{}, fmt.Errorf("%w: startup artifact is not structured", ErrManualDetachSource)
	}
	head, err := application.database.Head(ctx)
	if err != nil {
		return ManualSave{}, err
	}
	if head == nil || head.ID != source.CanonicalRevisionID {
		return ManualSave{}, fmt.Errorf("%w: source canonical revision is not the current head", ErrManualDetachSource)
	}
	return application.ReplaceManualJSON(ctx, ManualReplaceRequest{
		ExpectedHead: head.ID, CoreVersion: source.ExactCoreVersion,
		CoreArtifactID: source.CoreArtifactID, Raw: append([]byte(nil), source.ConfigBytes...),
	})
}

// PreviewManualReplace parses and binds exact JSONC bytes without writing. A
// usable exact pinned manifest contributes only losslessly reversible owned
// paths; every other leaf is reported as residual manual ownership.
func (application *Application) PreviewManualReplace(
	ctx context.Context,
	request ManualReplaceRequest,
) (ManualReplacePreview, error) {
	prepared, err := application.prepareManualReplace(ctx, request)
	if err != nil {
		return ManualReplacePreview{}, err
	}
	return prepared.preview, nil
}

// ReplaceManualJSON preserves the exact JSONC bytes and atomically binds them
// to one canonical base, exact version, and immutable binary. Before writing,
// it computes the same reverse preview exposed by PreviewManualReplace. Without
// proof from a usable exact pinned manifest, canonical intentionally remains
// unchanged and every version-specific path stays owned by the manual artifact.
func (application *Application) ReplaceManualJSON(
	ctx context.Context,
	request ManualReplaceRequest,
) (ManualSave, error) {
	prepared, err := application.prepareManualReplace(ctx, request)
	if err != nil {
		return ManualSave{}, err
	}
	return application.commitPreparedManualReplace(ctx, prepared)
}

func (application *Application) commitPreparedManualReplace(
	ctx context.Context,
	prepared preparedManualReplace,
) (ManualSave, error) {
	request := prepared.request
	revisionID, err := application.newID("rev")
	if err != nil {
		return ManualSave{}, err
	}
	commandID, err := application.newID("cmd")
	if err != nil {
		return ManualSave{}, err
	}
	startupID, err := application.newID("startup")
	if err != nil {
		return ManualSave{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return ManualSave{}, err
	}
	createdAt := application.now().UTC()
	payload, err := json.Marshal(map[string]string{"startup_artifact_id": startupID})
	if err != nil {
		return ManualSave{}, err
	}
	save := func() (store.SaveCanonicalManualArtifactResult, error) {
		diagnostics, diagnosticsErr := manualReplaceDiagnostics(prepared.preview.Reverse)
		if diagnosticsErr != nil {
			return store.SaveCanonicalManualArtifactResult{}, diagnosticsErr
		}
		var projectionEvidence *store.ManualProjectionEvidence
		if prepared.preview.Reverse.Available {
			pin := prepared.preview.Reverse.Capability
			if pin == nil {
				return store.SaveCanonicalManualArtifactResult{}, errors.New(
					"available manual reverse projection has no capability evidence",
				)
			}
			projectionEvidence = &store.ManualProjectionEvidence{
				ExactCoreVersion: prepared.preview.Resolution.ExactVersion,
				Repository:       pin.Repository,
				CommitSHA:        pin.CommitSHA,
				ManifestSHA256:   pin.ManifestSHA256,
				SupportLevel:     pin.SupportLevel,
			}
		}
		return application.database.SaveCanonicalManualArtifactAndTask(ctx, store.SaveCanonicalManualArtifactInput{
			ExpectedHead:       request.ExpectedHead,
			ProjectionEvidence: projectionEvidence,
			Revision: store.NewCanonicalRevision{
				ID: revisionID, SchemaVersion: canonical.SchemaVersion,
				Document: prepared.canonicalDocument, CommandID: commandID, CreatedAt: createdAt,
			},
			Artifact: store.NewManualStartupArtifact{
				ID: startupID, ExactCoreVersion: prepared.preview.Resolution.ExactVersion,
				RendererVersion: "manual-json-v2", CoreArtifactID: prepared.core.ID,
				ConfigBytes: prepared.document.RawBytes(), ConfigSHA256: prepared.document.SHA256(),
				Diagnostics: diagnostics, CreatedAt: createdAt,
			},
			CheckTask: store.NewTask{
				ID: taskID, IdempotencyKey: "startup-check:" + startupID,
				Lane: store.TaskLaneMaintenance, Kind: "startup-check",
				Payload: payload, CreatedAt: createdAt,
			},
		})
	}

	stored, err := save()
	if reason, fallback := manualProjectionFallbackReason(err); fallback {
		prepared.preview.Reverse = ManualReverseMapping{
			ReasonCode:        reason,
			OwnedPartial:      json.RawMessage(`{}`),
			ProposedCanonical: bytes.Clone(prepared.preview.Base.Document),
			ResidualPaths:     manualLeafPointers(prepared.document.Object()),
		}
		prepared.canonicalDocument = bytes.Clone(prepared.preview.Base.Document)
		stored, err = save()
	}
	if err != nil {
		return ManualSave{}, err
	}
	return ManualSave{
		Resolution: prepared.preview.Resolution, Preview: prepared.preview,
		Revision: snapshot(stored.Revision), NoChange: stored.NoChange,
		Artifact: manualArtifact(stored.StartupArtifact), Task: applicationTask(stored.CheckTask),
	}, nil
}

func manualProjectionFallbackReason(err error) (string, bool) {
	switch {
	case errors.Is(err, store.ErrCapabilityManifestQuarantined):
		return "capability_quarantined", true
	case errors.Is(err, store.ErrManualProjectionEvidenceStale):
		return "capability_pin_changed", true
	default:
		return "", false
	}
}

func (application *Application) prepareManualReplace(
	ctx context.Context,
	request ManualReplaceRequest,
) (preparedManualReplace, error) {
	request.ExpectedHead = strings.TrimSpace(request.ExpectedHead)
	if request.ExpectedHead == "" {
		return preparedManualReplace{}, errors.New("manual JSON requires an existing base revision")
	}
	resolution, err := application.ResolveCoreVersion(ctx, request.CoreVersion)
	if err != nil {
		return preparedManualReplace{}, err
	}
	core, err := application.resolveCoreArtifact(ctx, resolution, strings.TrimSpace(request.CoreArtifactID))
	if err != nil {
		return preparedManualReplace{}, err
	}
	head, err := application.database.Head(ctx)
	if err != nil {
		return preparedManualReplace{}, err
	}
	if head == nil || head.ID != request.ExpectedHead {
		actual := ""
		if head != nil {
			actual = head.ID
		}
		return preparedManualReplace{}, &store.RevisionConflictError{ExpectedHead: request.ExpectedHead, ActualHead: actual}
	}
	baseDocument, err := canonical.Parse(head.Document)
	if err != nil {
		return preparedManualReplace{}, err
	}
	version, err := coreartifact.ParseExactVersion(resolution.ExactVersion)
	if err != nil {
		return preparedManualReplace{}, err
	}
	binaryDigest, err := coreartifact.ParseSHA256(core.BinarySHA256)
	if err != nil {
		return preparedManualReplace{}, err
	}
	document, err := manualjson.Parse(request.Raw, manualjson.Binding{
		CoreVersion: version, ArtifactDigest: binaryDigest, BaseRevisionID: head.ID,
	})
	if err != nil {
		return preparedManualReplace{}, err
	}

	reverse := ManualReverseMapping{
		ReasonCode:        "capability_pin_unavailable",
		OwnedPartial:      json.RawMessage(`{}`),
		ProposedCanonical: bytes.Clone(baseDocument.CanonicalJSON()),
		ResidualPaths:     manualLeafPointers(document.Object()),
	}
	manifest, pin, pinErr := application.PinnedCapabilityManifest(ctx, resolution.ExactVersion)
	switch {
	case errors.Is(pinErr, store.ErrCapabilityPinNotFound):
	case errors.Is(pinErr, ErrCapabilityCandidateQuarantined):
		reverse.ReasonCode = "capability_quarantined"
	case pinErr != nil:
		return preparedManualReplace{}, pinErr
	case manifest.CoreVersion().String() != resolution.ExactVersion:
		reverse.ReasonCode = "capability_identity_invalid"
	case manifest.SupportLevel() == capability.SupportCompatibleStructured && !request.AllowCompatible:
		reverse.ReasonCode = "compatible_acceptance_required"
	case !structuredSupportLevel(manifest.SupportLevel()):
		reverse.ReasonCode = "structured_capability_unavailable"
	default:
		projector, projectorErr := capability.NewProjector(manifest)
		if projectorErr != nil {
			reverse.ReasonCode = "reverse_projection_not_proven"
			break
		}
		partial, reverseErr := projector.ReversePartial(document.Object())
		if reverseErr != nil {
			reverse.ReasonCode = "reverse_projection_not_proven"
			break
		}
		proposed, overlayErr := overlayCanonical(baseDocument.Map(), partial.Canonical)
		if overlayErr != nil {
			reverse.ReasonCode = "reverse_projection_not_proven"
			break
		}
		proposedJSON, encodeErr := canonicalMapJSON(proposed)
		if encodeErr != nil {
			reverse.ReasonCode = "reverse_projection_not_proven"
			break
		}
		ownedJSON, encodeErr := marshalObject(partial.Canonical)
		if encodeErr != nil {
			return preparedManualReplace{}, encodeErr
		}
		residual := append([]string(nil), partial.ResidualPaths...)
		sort.Strings(residual)
		reverse = ManualReverseMapping{
			Available: true,
			Capability: &ManualReattachPinEvidence{
				Repository: pin.Repository, CommitSHA: pin.CommitSHA,
				ManifestSHA256: pin.ManifestSHA256, SupportLevel: pin.SupportLevel,
			},
			OwnedPartial: ownedJSON, ProposedCanonical: proposedJSON,
			ResidualPaths:    residual,
			CanonicalChanged: !bytes.Equal(proposedJSON, baseDocument.CanonicalJSON()),
		}
	}

	preview := ManualReplacePreview{
		Resolution: resolution, Base: snapshot(*head), CoreArtifactID: core.ID,
		ConfigSHA256: document.SHA256(), Reverse: reverse,
	}
	return preparedManualReplace{
		request: request, core: core, document: document,
		canonicalDocument: bytes.Clone(reverse.ProposedCanonical), preview: preview,
	}, nil
}

func manualReplaceDiagnostics(reverse ManualReverseMapping) (json.RawMessage, error) {
	value := map[string]any{
		"code":              "manual_json_exact_bytes",
		"reverse_available": reverse.Available,
		"residual_owner":    "manual_artifact",
		"residual_paths":    reverse.ResidualPaths,
	}
	if reverse.ReasonCode != "" {
		value["reverse_reason_code"] = reverse.ReasonCode
	}
	if reverse.Capability != nil {
		value["capability_repository"] = reverse.Capability.Repository
		value["capability_commit"] = reverse.Capability.CommitSHA
		value["manifest_sha256"] = reverse.Capability.ManifestSHA256
		value["canonical_changed"] = reverse.CanonicalChanged
	}
	encoded, err := json.Marshal([]map[string]any{value})
	if err != nil {
		return nil, fmt.Errorf("encode manual reverse diagnostics: %w", err)
	}
	return encoded, nil
}

func manualLeafPointers(document map[string]any) []string {
	paths := make([]string, 0)
	var collect func(any, string)
	collect = func(value any, path string) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if len(keys) == 0 && path != "" {
				paths = append(paths, path)
			}
			for _, key := range keys {
				collect(typed[key], path+"/"+escapePointerToken(key))
			}
		case []any:
			if len(typed) == 0 && path != "" {
				paths = append(paths, path)
			}
			for index, child := range typed {
				collect(child, path+"/"+strconv.Itoa(index))
			}
		default:
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	collect(document, "")
	sort.Strings(paths)
	return paths
}

func (application *Application) QueueStartupCheck(
	ctx context.Context,
	startupArtifactID string,
) (Task, error) {
	artifact, err := application.database.GetStartupArtifact(ctx, startupArtifactID)
	if err != nil {
		return Task{}, err
	}
	if artifact.State != store.StartupArtifactPending {
		return Task{}, fmt.Errorf(
			"%w: startup artifact %s is %s, not pending",
			store.ErrStartupArtifactState,
			artifact.ID,
			artifact.State,
		)
	}
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	payload, err := json.Marshal(map[string]string{"startup_artifact_id": artifact.ID})
	if err != nil {
		return Task{}, err
	}
	queued, err := application.database.EnqueueTask(ctx, store.EnqueueTaskInput{
		ID: taskID, IdempotencyKey: "startup-check:" + artifact.ID,
		Lane: store.TaskLaneMaintenance, Kind: "startup-check",
		CanonicalRevisionID: artifact.CanonicalRevisionID, StartupArtifactID: artifact.ID,
		Payload: payload, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(queued), nil
}

func (application *Application) ManualArtifact(
	ctx context.Context,
	startupArtifactID string,
) (ManualArtifact, error) {
	artifact, err := application.database.GetStartupArtifact(ctx, startupArtifactID)
	if err != nil {
		return ManualArtifact{}, err
	}
	if artifact.Kind != store.StartupArtifactManual {
		return ManualArtifact{}, errors.New("startup artifact is not manual JSON")
	}
	return manualArtifact(artifact), nil
}

func (application *Application) ListManualArtifacts(
	ctx context.Context,
	explicitVersion string,
	coreArtifactID string,
	limit int,
) (CoreVersionResolution, []ManualArtifact, error) {
	resolution, err := application.ResolveCoreVersion(ctx, explicitVersion)
	if err != nil {
		return CoreVersionResolution{}, nil, err
	}
	page, err := application.database.ListStartupArtifacts(ctx, store.StartupArtifactListFilter{
		ExactCoreVersion: resolution.ExactVersion, CoreArtifactID: strings.TrimSpace(coreArtifactID),
		Kind: store.StartupArtifactManual, Limit: limit,
	})
	if err != nil {
		return CoreVersionResolution{}, nil, err
	}
	result := make([]ManualArtifact, len(page.Items))
	for index, artifact := range page.Items {
		result[index] = manualArtifact(artifact)
	}
	return resolution, result, nil
}

func (application *Application) DiscardManualArtifact(
	ctx context.Context,
	startupArtifactID string,
) (ManualArtifact, error) {
	current, err := application.database.GetStartupArtifact(ctx, startupArtifactID)
	if err != nil {
		return ManualArtifact{}, err
	}
	if current.Kind != store.StartupArtifactManual {
		return ManualArtifact{}, errors.New("startup artifact is not manual JSON")
	}
	stored, err := application.database.MarkStartupArtifactStale(ctx, startupArtifactID)
	if err != nil {
		return ManualArtifact{}, err
	}
	return manualArtifact(stored), nil
}

func (application *Application) resolveCoreArtifact(
	ctx context.Context,
	resolution CoreVersionResolution,
	artifactID string,
) (store.CoreArtifact, error) {
	if artifactID == "" && resolution.Running != nil {
		artifactID = resolution.Running.CoreArtifactID
	}
	if artifactID != "" {
		artifact, err := application.database.GetCoreArtifact(ctx, artifactID)
		if err != nil {
			return store.CoreArtifact{}, err
		}
		if artifact.ExactVersion != resolution.ExactVersion ||
			artifact.VerificationState != store.CoreArtifactVerified {
			return store.CoreArtifact{}, errors.New("core artifact is not a verified artifact for the resolved exact version")
		}
		return artifact, nil
	}
	page, err := application.database.ListCoreArtifacts(ctx, store.CoreArtifactListFilter{
		ExactVersion: resolution.ExactVersion, VerificationState: store.CoreArtifactVerified, Limit: 3,
	})
	if err != nil {
		return store.CoreArtifact{}, err
	}
	if len(page.Items) == 0 {
		return store.CoreArtifact{}, errors.New("no verified core artifact is installed for the resolved exact version")
	}
	if len(page.Items) != 1 {
		return store.CoreArtifact{}, fmt.Errorf(
			"multiple verified artifacts are installed for %s; select an exact artifact",
			resolution.ExactVersion,
		)
	}
	return page.Items[0], nil
}

func manualArtifact(value store.StartupArtifact) ManualArtifact {
	return ManualArtifact{
		ID: value.ID, CanonicalRevisionID: value.CanonicalRevisionID,
		ExactCoreVersion: value.ExactCoreVersion, CoreArtifactID: value.CoreArtifactID,
		ConfigSHA256: value.ConfigSHA256, Raw: string(value.ConfigBytes),
		Diagnostics: append(json.RawMessage(nil), value.Diagnostics...), State: value.State,
		CheckedAt: cloneTime(value.CheckedAt), CreatedAt: value.CreatedAt,
	}
}

func IsInvalidManualJSON(err error) bool {
	return errors.Is(err, manualjson.ErrInvalidManualJSON)
}

func IsManualDetachSourceInvalid(err error) bool {
	return errors.Is(err, ErrManualDetachSource)
}

func IsStartupArtifactNotFound(err error) bool {
	return errors.Is(err, store.ErrStartupArtifactNotFound)
}
