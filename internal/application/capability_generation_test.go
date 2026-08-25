// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCapabilityRefreshPreviewUpgradeAndOfflinePin(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	now := time.Date(2026, time.August, 26, 22, 0, 0, 0, time.UTC)
	application.now = func() time.Time { return now }
	commit := strings.Repeat("c", 40)
	source, digest := applicationCapabilityGeneration(t, commit, "1.13.19")

	refreshed, err := application.RefreshCapabilityGeneration(ctx, source)
	if err != nil {
		t.Fatalf("RefreshCapabilityGeneration: %v", err)
	}
	if !refreshed.Created || refreshed.Generation.CommitSHA != commit || len(refreshed.Candidates) != 1 {
		t.Fatalf("refreshed = %+v", refreshed)
	}
	status, err := application.CoreCapabilityStatus(ctx, "1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	if status.Pinned || status.SupportLevel != capability.SupportManualJSON {
		t.Fatalf("refresh implicitly changed support: %+v", status)
	}

	request := CapabilityUpgradeRequest{
		ExactCoreVersion: "1.13.19",
		CommitSHA:        commit,
		ManifestSHA256:   digest,
	}
	preview, err := application.PreviewCapabilityUpgrade(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Current != nil || !preview.Changed || preview.Blocked ||
		preview.Candidate.ManifestSHA256 != digest || len(preview.Candidate.Manifest) == 0 {
		t.Fatalf("preview = %+v", preview)
	}
	upgraded, err := application.UpgradeCapability(ctx, request)
	if err != nil {
		t.Fatalf("UpgradeCapability: %v", err)
	}
	if upgraded.Pin.ManifestSHA256 != digest || upgraded.Pin.ExactCoreVersion != "1.13.19" {
		t.Fatalf("upgrade = %+v", upgraded)
	}
	status, err = application.CoreCapabilityStatus(ctx, "1.13.19")
	if err != nil || !status.Pinned || status.SupportLevel != capability.SupportNativeStructured {
		t.Fatalf("pinned status = %+v, %v", status, err)
	}
	if status.Presentation == nil || len(status.Presentation.SemanticFacts) != 1 ||
		len(status.Presentation.UI) != 1 || status.Presentation.UI[0].FactID != "route.mode" {
		t.Fatalf("pinned presentation = %+v", status.Presentation)
	}
	manifest, pin, err := application.PinnedCapabilityManifest(ctx, "1.13.19")
	if err != nil {
		t.Fatalf("PinnedCapabilityManifest without network: %v", err)
	}
	if manifest.CoreVersion().String() != "1.13.19" || pin.CommitSHA != commit {
		t.Fatalf("offline manifest/pin = %s / %+v", manifest.CoreVersion(), pin)
	}
	if _, err := database.UpsertCapabilityQuarantine(ctx, store.CapabilityQuarantine{
		ManifestSHA256: digest,
		ReasonCode:     "round_trip_failed",
		Diagnostics:    json.RawMessage(`{"fixture":"projection"}`),
	}); err != nil {
		t.Fatal(err)
	}
	status, err = application.CoreCapabilityStatus(ctx, "1.13.19")
	if err != nil || !status.Quarantined || status.SupportLevel != capability.SupportManualJSON {
		t.Fatalf("quarantined effective status = %+v, %v", status, err)
	}
	if status.Presentation != nil {
		t.Fatalf("quarantined status exposed presentation = %+v", status.Presentation)
	}
	if _, _, err := application.PinnedCapabilityManifest(ctx, "1.13.19"); !errors.Is(err, ErrCapabilityCandidateQuarantined) {
		t.Fatalf("quarantined pinned manifest error = %v", err)
	}
}

func TestCapabilityInvalidRefreshIsQuarantinedAndDoesNotReplaceLKG(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	application.now = func() time.Time {
		return time.Date(2026, time.August, 26, 23, 0, 0, 0, time.UTC)
	}
	commit := strings.Repeat("d", 40)
	valid, digest := applicationCapabilityGeneration(t, commit, "1.13.19")
	if _, err := application.RefreshCapabilityGeneration(ctx, valid); err != nil {
		t.Fatal(err)
	}
	request := CapabilityUpgradeRequest{ExactCoreVersion: "1.13.19", CommitSHA: commit, ManifestSHA256: digest}
	if _, err := application.UpgradeCapability(ctx, request); err != nil {
		t.Fatal(err)
	}

	invalid := []byte(`{"schema_version":1}`)
	if _, err := application.RefreshCapabilityGeneration(ctx, invalid); !errors.Is(err, capability.ErrInvalidGeneration) {
		t.Fatalf("invalid refresh error = %v", err)
	}
	quarantine, err := database.GetCapabilityQuarantine(ctx, sourceDigest(invalid))
	if err != nil || quarantine.ReasonCode != "generation_invalid" {
		t.Fatalf("invalid source quarantine = %+v, %v", quarantine, err)
	}
	manifest, pin, err := application.PinnedCapabilityManifest(ctx, "1.13.19")
	if err != nil || manifest.CoreVersion().String() != "1.13.19" || pin.ManifestSHA256 != digest {
		t.Fatalf("last-known-good was not retained: manifest=%v pin=%+v err=%v", manifest, pin, err)
	}
}

func TestCapabilityStatusNeverPresentsManualUnavailableOrUnpinnedVersions(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)

	for index, level := range []capability.SupportLevel{
		capability.SupportManualJSON,
		capability.SupportUnavailable,
	} {
		version := []string{"1.13.18", "1.13.17"}[index]
		commit := strings.Repeat(string(rune('a'+index)), 40)
		source, digest := applicationCapabilityGenerationWithSupport(t, commit, version, level)
		if _, err := application.RefreshCapabilityGeneration(ctx, source); err != nil {
			t.Fatal(err)
		}
		if _, err := application.UpgradeCapability(ctx, CapabilityUpgradeRequest{
			ExactCoreVersion: version, CommitSHA: commit, ManifestSHA256: digest,
		}); err != nil {
			t.Fatal(err)
		}
		status, err := application.CoreCapabilityStatus(ctx, version)
		if err != nil || status.SupportLevel != level || status.Presentation != nil {
			t.Fatalf("status(%s) = %+v, %v", level, status, err)
		}
	}

	status, err := application.CoreCapabilityStatus(ctx, "1.13.16")
	if err != nil || status.Pinned || status.Presentation != nil {
		t.Fatalf("unpinned status = %+v, %v", status, err)
	}
}

func TestCapabilityManifestQuarantineIsPermanentAuditableAndIdempotent(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	quarantinedAt := time.Date(2026, time.August, 26, 23, 30, 0, 0, time.UTC)
	application.now = func() time.Time { return quarantinedAt }
	digest := strings.Repeat("a", 64)

	first, err := application.QuarantineCapabilityManifest(ctx, CapabilityQuarantineRequest{
		ManifestSHA256: digest,
		ReasonCode:     "operator_validation_failed",
	})
	if err != nil {
		t.Fatalf("QuarantineCapabilityManifest() error = %v", err)
	}
	if first.ManifestSHA256 != digest || first.ReasonCode != "operator_validation_failed" ||
		first.QuarantinedAt != quarantinedAt {
		t.Fatalf("quarantine = %+v", first)
	}

	application.now = func() time.Time { return quarantinedAt.Add(time.Hour) }
	retry, err := application.QuarantineCapabilityManifest(ctx, CapabilityQuarantineRequest{
		ManifestSHA256: strings.ToUpper(digest),
		ReasonCode:     "operator_validation_failed",
	})
	if err != nil || retry != first {
		t.Fatalf("idempotent quarantine = %+v, %v; want %+v", retry, err, first)
	}
	if _, err := application.QuarantineCapabilityManifest(ctx, CapabilityQuarantineRequest{
		ManifestSHA256: digest,
		ReasonCode:     "security_advisory",
	}); !errors.Is(err, ErrCapabilityQuarantineConflict) {
		t.Fatalf("changed quarantine reason error = %v", err)
	}
	stored, err := database.GetCapabilityQuarantine(ctx, digest)
	if err != nil || stored.ReasonCode != first.ReasonCode || stored.QuarantinedAt != first.QuarantinedAt ||
		string(stored.Diagnostics) != `{"source":"administrator"}` {
		t.Fatalf("stored audit record = %+v, %v", stored, err)
	}
}

func TestCapabilityManifestQuarantineRejectsUnstableInputs(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)

	for _, request := range []CapabilityQuarantineRequest{
		{ManifestSHA256: "short", ReasonCode: "security_advisory"},
		{ManifestSHA256: strings.Repeat("0", 64), ReasonCode: "security_advisory"},
		{ManifestSHA256: strings.Repeat("a", 64), ReasonCode: "Security advisory"},
		{ManifestSHA256: strings.Repeat("a", 64), ReasonCode: "_reason"},
	} {
		if _, err := application.QuarantineCapabilityManifest(ctx, request); !errors.Is(err, ErrCapabilityQuarantineInvalid) {
			t.Fatalf("QuarantineCapabilityManifest(%+v) error = %v", request, err)
		}
	}
}

func TestCapabilityUpgradeRequiresExactImmutableCandidate(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	commit := strings.Repeat("e", 40)
	source, digest := applicationCapabilityGeneration(t, commit, "1.13.19")
	if _, err := application.RefreshCapabilityGeneration(ctx, source); err != nil {
		t.Fatal(err)
	}

	requests := []CapabilityUpgradeRequest{
		{},
		{ExactCoreVersion: "1.13.19", CommitSHA: commit, ManifestSHA256: strings.Repeat("a", 64)},
		{ExactCoreVersion: "1.13.18", CommitSHA: commit, ManifestSHA256: digest},
		{ExactCoreVersion: "1.13.19", CommitSHA: strings.Repeat("f", 40), ManifestSHA256: digest},
	}
	for index, request := range requests {
		if _, err := application.UpgradeCapability(ctx, request); err == nil {
			t.Fatalf("invalid upgrade[%d] succeeded", index)
		}
	}
	if _, err := database.GetCapabilityPin(ctx, "1.13.19"); !errors.Is(err, store.ErrCapabilityPinNotFound) {
		t.Fatalf("failed upgrade moved pin: %v", err)
	}
}

func applicationCapabilityGeneration(t *testing.T, commit, exactVersion string) ([]byte, string) {
	return applicationCapabilityGenerationWithSupport(
		t,
		commit,
		exactVersion,
		capability.SupportNativeStructured,
	)
}

func applicationCapabilityGenerationWithSupport(
	t *testing.T,
	commit string,
	exactVersion string,
	support capability.SupportLevel,
) ([]byte, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	spec := capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  support,
	}
	if support == capability.SupportNativeStructured ||
		support == capability.SupportCompatibleStructured {
		spec.SemanticFacts = []capability.SemanticFact{{
			ID:             "route.mode",
			CanonicalPath:  "/route/mode",
			Classification: capability.CoverageSupported,
			OwnedPaths:     []string{"/route_mode"},
		}}
		spec.Transforms = []capability.Transform{{
			ID:        "route.mode.enum",
			FactID:    "route.mode",
			Primitive: capability.PrimitiveEnum,
			From:      []string{"/route/mode"},
			To:        []string{"/route_mode"},
			Enum:      map[string]string{"direct": "direct", "block": "reject"},
		}}
		spec.UI = []capability.UIDescriptor{{
			ID:     "route.mode.select",
			FactID: "route.mode",
			Kind:   capability.UISelect,
			Label:  "Route mode",
			Help:   "Controls the exact-version route behavior.",
			Order:  10,
			Options: []capability.UIOption{
				{Value: "direct", Label: "Direct"},
				{Value: "block", Label: "Block"},
			},
		}}
	}
	manifest, err := capability.NewManifest(spec)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	value := struct {
		SchemaVersion uint32 `json:"schema_version"`
		Repository    string `json:"repository"`
		CommitSHA     string `json:"commit_sha"`
		ManifestCount int    `json:"manifest_count"`
		Manifests     []struct {
			Path           string          `json:"path"`
			ManifestSHA256 string          `json:"manifest_sha256"`
			Manifest       json.RawMessage `json:"manifest"`
		} `json:"manifests"`
	}{
		SchemaVersion: capability.GenerationSchemaVersion,
		Repository:    capability.ManifestRepository,
		CommitSHA:     commit,
		ManifestCount: 1,
	}
	value.Manifests = append(value.Manifests, struct {
		Path           string          `json:"path"`
		ManifestSHA256 string          `json:"manifest_sha256"`
		Manifest       json.RawMessage `json:"manifest"`
	}{
		Path: "capabilities/" + exactVersion + ".json", ManifestSHA256: digest.String(), Manifest: canonical,
	})
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, digest.String()
}
