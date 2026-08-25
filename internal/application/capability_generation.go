// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var (
	ErrCapabilityCandidateQuarantined = errors.New("capability candidate is quarantined")
	ErrCapabilityQuarantineInvalid    = errors.New("capability quarantine request is invalid")
	ErrCapabilityQuarantineConflict   = errors.New("capability manifest is already quarantined with a different reason")
)

type CapabilityGenerationView struct {
	ID            string    `json:"id"`
	Repository    string    `json:"repository"`
	CommitSHA     string    `json:"commit_sha"`
	SourceSHA256  string    `json:"source_sha256"`
	ManifestCount int       `json:"manifest_count"`
	RefreshedAt   time.Time `json:"refreshed_at"`
}

type CapabilityManifestCandidate struct {
	GenerationID     string                  `json:"generation_id"`
	Repository       string                  `json:"repository"`
	CommitSHA        string                  `json:"commit_sha"`
	ExactCoreVersion string                  `json:"exact_core_version"`
	Path             string                  `json:"path"`
	ManifestSHA256   string                  `json:"manifest_sha256"`
	SupportLevel     capability.SupportLevel `json:"support_level"`
	Manifest         json.RawMessage         `json:"manifest,omitempty"`
	Quarantined      bool                    `json:"quarantined"`
	ReasonCode       string                  `json:"reason_code,omitempty"`
}

type CapabilityGenerationRefresh struct {
	Generation CapabilityGenerationView      `json:"generation"`
	Candidates []CapabilityManifestCandidate `json:"candidates"`
	Created    bool                          `json:"created"`
}

type CapabilityPinView struct {
	ExactCoreVersion string                  `json:"exact_core_version"`
	Repository       string                  `json:"repository"`
	CommitSHA        string                  `json:"commit_sha"`
	ManifestSHA256   string                  `json:"manifest_sha256"`
	SupportLevel     capability.SupportLevel `json:"support_level"`
	PinnedAt         time.Time               `json:"pinned_at"`
}

type CapabilityUpgradeRequest struct {
	ExactCoreVersion string
	CommitSHA        string
	ManifestSHA256   string
}

type CapabilityUpgradePreview struct {
	Current     *CapabilityPinView          `json:"current,omitempty"`
	Candidate   CapabilityManifestCandidate `json:"candidate"`
	Changed     bool                        `json:"changed"`
	Blocked     bool                        `json:"blocked"`
	BlockReason string                      `json:"block_reason,omitempty"`
	Warnings    []string                    `json:"warnings"`
}

type CapabilityUpgrade struct {
	Preview CapabilityUpgradePreview `json:"preview"`
	Pin     CapabilityPinView        `json:"pin"`
}

type CapabilityQuarantineRequest struct {
	ManifestSHA256 string
	ReasonCode     string
}

type CapabilityQuarantineView struct {
	ManifestSHA256 string    `json:"manifest_sha256"`
	ReasonCode     string    `json:"reason_code"`
	QuarantinedAt  time.Time `json:"quarantined_at"`
}

// QuarantineCapabilityManifest permanently blocks one immutable manifest
// digest from new pins, projections, and applies. Repeating the same reason is
// idempotent; a different reason cannot rewrite the original audit record.
func (application *Application) QuarantineCapabilityManifest(
	ctx context.Context,
	request CapabilityQuarantineRequest,
) (CapabilityQuarantineView, error) {
	digest, err := coreartifact.ParseSHA256(request.ManifestSHA256)
	if err != nil || digest.IsZero() {
		return CapabilityQuarantineView{}, fmt.Errorf(
			"%w: manifest SHA-256 must be a non-zero 64-character digest",
			ErrCapabilityQuarantineInvalid,
		)
	}
	if !validCapabilityQuarantineReason(request.ReasonCode) {
		return CapabilityQuarantineView{}, fmt.Errorf(
			"%w: reason code must be 3-64 lowercase letters, digits, or underscores and begin and end with a letter or digit",
			ErrCapabilityQuarantineInvalid,
		)
	}
	stored, err := application.database.UpsertCapabilityQuarantine(ctx, store.CapabilityQuarantine{
		ManifestSHA256: digest.String(),
		ReasonCode:     request.ReasonCode,
		Diagnostics:    json.RawMessage(`{"source":"administrator"}`),
		QuarantinedAt:  application.now().UTC(),
	})
	if err != nil {
		return CapabilityQuarantineView{}, err
	}
	if stored.ReasonCode != request.ReasonCode {
		return CapabilityQuarantineView{}, fmt.Errorf(
			"%w: %s is recorded as %s",
			ErrCapabilityQuarantineConflict,
			stored.ManifestSHA256,
			stored.ReasonCode,
		)
	}
	return capabilityQuarantineView(stored), nil
}

func validCapabilityQuarantineReason(reason string) bool {
	if len(reason) < 3 || len(reason) > 64 || !lowercaseLetterOrDigit(reason[0]) ||
		!lowercaseLetterOrDigit(reason[len(reason)-1]) {
		return false
	}
	for index := 1; index < len(reason)-1; index++ {
		character := reason[index]
		if !lowercaseLetterOrDigit(character) && character != '_' {
			return false
		}
	}
	return true
}

func lowercaseLetterOrDigit(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}

// RefreshCapabilityGeneration accepts a complete local, commit-bound
// generation as a candidate. It deliberately never changes capability pins.
func (application *Application) RefreshCapabilityGeneration(
	ctx context.Context,
	source []byte,
) (CapabilityGenerationRefresh, error) {
	generation, err := capability.DecodeGeneration(source)
	if err != nil {
		return CapabilityGenerationRefresh{}, errors.Join(
			err,
			application.quarantineCapabilityObject(ctx, sourceDigest(source), "generation_invalid", err),
		)
	}

	for _, entry := range generation.Manifests() {
		quarantine, lookupErr := application.database.GetCapabilityQuarantine(ctx, entry.Digest().String())
		if lookupErr == nil {
			return CapabilityGenerationRefresh{}, fmt.Errorf(
				"%w: %s (%s)",
				ErrCapabilityCandidateQuarantined,
				entry.Digest(),
				quarantine.ReasonCode,
			)
		}
		if !errors.Is(lookupErr, store.ErrCapabilityQuarantineNotFound) {
			return CapabilityGenerationRefresh{}, lookupErr
		}
	}

	saved, err := application.database.SaveCapabilityGeneration(ctx, source, application.now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrCapabilityGenerationConflict) {
			digest, digestErr := generation.Digest()
			if digestErr == nil {
				err = errors.Join(
					err,
					application.quarantineCapabilityObject(
						ctx,
						digest.String(),
						"generation_commit_conflict",
						err,
					),
				)
			}
		}
		return CapabilityGenerationRefresh{}, err
	}
	return capabilityGenerationRefresh(saved), nil
}

func (application *Application) ListCapabilityGenerations(
	ctx context.Context,
	limit int,
) ([]CapabilityGenerationView, error) {
	generations, err := application.database.ListCapabilityGenerations(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]CapabilityGenerationView, len(generations))
	for index, generation := range generations {
		result[index] = capabilityGenerationView(generation)
	}
	return result, nil
}

func (application *Application) InspectCapabilityCandidate(
	ctx context.Context,
	request CapabilityUpgradeRequest,
) (CapabilityManifestCandidate, error) {
	stored, manifest, err := application.loadCapabilityCandidate(ctx, request)
	if err != nil {
		return CapabilityManifestCandidate{}, err
	}
	result := capabilityManifestCandidate(stored, true)
	canonical, err := manifest.CanonicalJSON()
	if err != nil {
		return CapabilityManifestCandidate{}, err
	}
	result.Manifest = canonical
	quarantine, err := application.database.GetCapabilityQuarantine(ctx, result.ManifestSHA256)
	if err == nil {
		result.Quarantined = true
		result.ReasonCode = quarantine.ReasonCode
		return result, nil
	}
	if !errors.Is(err, store.ErrCapabilityQuarantineNotFound) {
		return CapabilityManifestCandidate{}, err
	}
	return result, nil
}

func (application *Application) PreviewCapabilityUpgrade(
	ctx context.Context,
	request CapabilityUpgradeRequest,
) (CapabilityUpgradePreview, error) {
	candidate, err := application.InspectCapabilityCandidate(ctx, request)
	if err != nil {
		return CapabilityUpgradePreview{}, err
	}
	preview := CapabilityUpgradePreview{
		Candidate: candidate,
		Changed:   true,
		Warnings:  make([]string, 0),
	}
	current, err := application.database.GetCapabilityPin(ctx, candidate.ExactCoreVersion)
	if err == nil {
		view := capabilityPinView(current)
		preview.Current = &view
		preview.Changed = current.Repository != candidate.Repository ||
			current.CommitSHA != candidate.CommitSHA ||
			current.ManifestSHA256 != candidate.ManifestSHA256 ||
			current.SupportLevel != candidate.SupportLevel
	} else if !errors.Is(err, store.ErrCapabilityPinNotFound) {
		return CapabilityUpgradePreview{}, err
	}

	if candidate.Quarantined {
		preview.Blocked = true
		preview.BlockReason = "candidate is quarantined: " + candidate.ReasonCode
	}
	switch candidate.SupportLevel {
	case capability.SupportCompatibleStructured:
		preview.Warnings = append(preview.Warnings, "compatible structured support requires explicit operator acceptance and remains warning-visible")
	case capability.SupportManualJSON:
		preview.Warnings = append(preview.Warnings, "this exact version remains in manual JSON mode")
	case capability.SupportUnavailable:
		preview.Warnings = append(preview.Warnings, "this manifest declares the exact version unavailable")
	}
	return preview, nil
}

// UpgradeCapability executes the explicit pin after producing the same preview
// returned by PreviewCapabilityUpgrade. The store rechecks candidate identity
// and quarantine state in the pin transaction.
func (application *Application) UpgradeCapability(
	ctx context.Context,
	request CapabilityUpgradeRequest,
) (CapabilityUpgrade, error) {
	preview, err := application.PreviewCapabilityUpgrade(ctx, request)
	if err != nil {
		return CapabilityUpgrade{}, err
	}
	if preview.Blocked {
		return CapabilityUpgrade{}, fmt.Errorf(
			"%w: %s",
			ErrCapabilityCandidateQuarantined,
			preview.BlockReason,
		)
	}
	pin, err := application.database.PinCapabilityGenerationManifest(
		ctx,
		request.CommitSHA,
		request.ExactCoreVersion,
		request.ManifestSHA256,
		application.now().UTC(),
	)
	if err != nil {
		return CapabilityUpgrade{}, err
	}
	return CapabilityUpgrade{Preview: preview, Pin: capabilityPinView(pin)}, nil
}

// PinnedCapabilityManifest resolves only locally persisted immutable content.
// It performs no network access, so last-known-good pins remain usable offline.
func (application *Application) PinnedCapabilityManifest(
	ctx context.Context,
	exactCoreVersion string,
) (*capability.Manifest, CapabilityPinView, error) {
	snapshot, err := application.database.PinnedCapability(ctx, exactCoreVersion)
	if err != nil {
		return nil, CapabilityPinView{}, err
	}
	if snapshot.Quarantine != nil {
		return nil, CapabilityPinView{}, fmt.Errorf(
			"%w: %s (%s)",
			ErrCapabilityCandidateQuarantined,
			snapshot.Pin.ManifestSHA256,
			snapshot.Quarantine.ReasonCode,
		)
	}
	manifest, err := decodeStoredCapabilityManifest(snapshot.Manifest)
	if err != nil {
		return nil, CapabilityPinView{}, err
	}
	return manifest, capabilityPinView(snapshot.Pin), nil
}

func (application *Application) loadCapabilityCandidate(
	ctx context.Context,
	request CapabilityUpgradeRequest,
) (store.CapabilityGenerationManifest, *capability.Manifest, error) {
	if request.ExactCoreVersion == "" || request.CommitSHA == "" || request.ManifestSHA256 == "" {
		return store.CapabilityGenerationManifest{}, nil, errors.New("exact core version, commit, and manifest SHA-256 are required")
	}
	stored, err := application.database.CapabilityGenerationManifest(
		ctx,
		request.CommitSHA,
		request.ExactCoreVersion,
		request.ManifestSHA256,
	)
	if err != nil {
		return store.CapabilityGenerationManifest{}, nil, err
	}
	manifest, err := decodeStoredCapabilityManifest(stored)
	if err != nil {
		return store.CapabilityGenerationManifest{}, nil, err
	}
	return stored, manifest, nil
}

func decodeStoredCapabilityManifest(
	stored store.CapabilityGenerationManifest,
) (*capability.Manifest, error) {
	manifest, err := capability.DecodeManifest(stored.ManifestJSON)
	if err != nil {
		return nil, fmt.Errorf("decode stored capability manifest: %w", err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		return nil, err
	}
	if digest.String() != stored.ManifestSHA256 ||
		manifest.CoreVersion().String() != stored.ExactCoreVersion ||
		manifest.SupportLevel() != stored.SupportLevel {
		return nil, errors.New("stored capability manifest identity is inconsistent")
	}
	return manifest, nil
}

func (application *Application) quarantineCapabilityObject(
	ctx context.Context,
	digest string,
	reasonCode string,
	cause error,
) error {
	diagnostics, err := json.Marshal(map[string]string{"error": cause.Error()})
	if err != nil {
		return err
	}
	_, err = application.database.UpsertCapabilityQuarantine(ctx, store.CapabilityQuarantine{
		ManifestSHA256: digest,
		ReasonCode:     reasonCode,
		Diagnostics:    diagnostics,
		QuarantinedAt:  application.now().UTC(),
	})
	return err
}

func sourceDigest(source []byte) string {
	return coreartifact.NewSHA256(sha256.Sum256(source)).String()
}

func capabilityGenerationRefresh(saved store.CapabilityGenerationSave) CapabilityGenerationRefresh {
	result := CapabilityGenerationRefresh{
		Generation: capabilityGenerationView(saved.Generation),
		Candidates: make([]CapabilityManifestCandidate, len(saved.Manifests)),
		Created:    saved.Created,
	}
	for index, candidate := range saved.Manifests {
		result.Candidates[index] = capabilityManifestCandidate(candidate, false)
	}
	return result
}

func capabilityGenerationView(value store.CapabilityGeneration) CapabilityGenerationView {
	return CapabilityGenerationView{
		ID:            value.ID,
		Repository:    value.Repository,
		CommitSHA:     value.CommitSHA,
		SourceSHA256:  value.SourceSHA256,
		ManifestCount: value.ManifestCount,
		RefreshedAt:   value.RefreshedAt,
	}
}

func capabilityManifestCandidate(
	value store.CapabilityGenerationManifest,
	includeManifest bool,
) CapabilityManifestCandidate {
	result := CapabilityManifestCandidate{
		GenerationID:     value.GenerationID,
		Repository:       value.Repository,
		CommitSHA:        value.CommitSHA,
		ExactCoreVersion: value.ExactCoreVersion,
		Path:             value.Path,
		ManifestSHA256:   value.ManifestSHA256,
		SupportLevel:     value.SupportLevel,
	}
	if includeManifest {
		result.Manifest = append(json.RawMessage(nil), value.ManifestJSON...)
	}
	return result
}

func capabilityPinView(value store.CapabilityPin) CapabilityPinView {
	return CapabilityPinView{
		ExactCoreVersion: value.ExactCoreVersion,
		Repository:       value.Repository,
		CommitSHA:        value.CommitSHA,
		ManifestSHA256:   value.ManifestSHA256,
		SupportLevel:     value.SupportLevel,
		PinnedAt:         value.PinnedAt,
	}
}

func capabilityQuarantineView(value store.CapabilityQuarantine) CapabilityQuarantineView {
	return CapabilityQuarantineView{
		ManifestSHA256: value.ManifestSHA256,
		ReasonCode:     value.ReasonCode,
		QuarantinedAt:  value.QuarantinedAt,
	}
}
