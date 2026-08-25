// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestCapabilityGenerationAtomicCandidateAndExplicitPin(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 21, 0, 0, 0, time.UTC)
	source, firstDigest := capabilityGenerationFixture(
		t,
		strings.Repeat("a", 40),
		[]capabilityGenerationManifestFixture{
			{version: "1.12.8", support: capability.SupportManualJSON},
			{version: "1.13.19", support: capability.SupportUnavailable},
		},
	)

	saved, err := database.SaveCapabilityGeneration(ctx, source, now)
	if err != nil {
		t.Fatalf("SaveCapabilityGeneration: %v", err)
	}
	if !saved.Created || saved.Generation.ManifestCount != 2 || len(saved.Manifests) != 2 {
		t.Fatalf("saved generation = %+v", saved)
	}
	if saved.Manifests[0].ExactCoreVersion != "1.12.8" ||
		saved.Manifests[0].ManifestSHA256 != firstDigest {
		t.Fatalf("first manifest = %+v", saved.Manifests[0])
	}

	retry, err := database.SaveCapabilityGeneration(ctx, source, now.Add(time.Minute))
	if err != nil || retry.Created || retry.Generation.ID != saved.Generation.ID {
		t.Fatalf("idempotent save = %+v, %v", retry, err)
	}
	candidate, err := database.CapabilityGenerationManifest(
		ctx,
		strings.Repeat("a", 40),
		"1.12.8",
		firstDigest,
	)
	if err != nil || candidate.SupportLevel != capability.SupportManualJSON {
		t.Fatalf("candidate = %+v, %v", candidate, err)
	}
	if _, err := database.GetCapabilityPin(ctx, "1.12.8"); !errors.Is(err, ErrCapabilityPinNotFound) {
		t.Fatalf("refresh moved pin: %v", err)
	}

	pin, err := database.PinCapabilityGenerationManifest(
		ctx,
		strings.Repeat("a", 40),
		"1.12.8",
		firstDigest,
		now.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("PinCapabilityGenerationManifest: %v", err)
	}
	if pin.Repository != capability.ManifestRepository || pin.CommitSHA != strings.Repeat("a", 40) ||
		pin.ManifestSHA256 != firstDigest || pin.SupportLevel != capability.SupportManualJSON {
		t.Fatalf("pin = %+v", pin)
	}
}

func TestCapabilityGenerationRejectsCommitConflictAndQuarantine(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	commit := strings.Repeat("b", 40)
	first, firstDigest := capabilityGenerationFixture(t, commit, []capabilityGenerationManifestFixture{
		{version: "1.13.19", support: capability.SupportManualJSON},
	})
	if _, err := database.SaveCapabilityGeneration(ctx, first, time.Now()); err != nil {
		t.Fatal(err)
	}
	conflict, _ := capabilityGenerationFixture(t, commit, []capabilityGenerationManifestFixture{
		{version: "1.13.19", support: capability.SupportUnavailable},
	})
	if _, err := database.SaveCapabilityGeneration(ctx, conflict, time.Now()); !errors.Is(err, ErrCapabilityGenerationConflict) {
		t.Fatalf("conflicting immutable commit error = %v", err)
	}

	if _, err := database.UpsertCapabilityQuarantine(ctx, CapabilityQuarantine{
		ManifestSHA256: firstDigest,
		ReasonCode:     "fixture_failed",
		Diagnostics:    json.RawMessage(`{"fixture":"round_trip"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.PinCapabilityGenerationManifest(
		ctx,
		commit,
		"1.13.19",
		firstDigest,
		time.Now(),
	); !errors.Is(err, ErrCapabilityManifestQuarantined) {
		t.Fatalf("quarantined pin error = %v", err)
	}
	if _, err := database.PinCapabilityGenerationManifest(
		ctx,
		commit,
		"1.13.18",
		firstDigest,
		time.Now(),
	); !errors.Is(err, ErrCapabilityManifestNotFound) {
		t.Fatalf("wrong exact version pin error = %v", err)
	}
	blockedCommit := strings.Repeat("c", 40)
	blockedSource, blockedDigest := capabilityGenerationFixture(t, blockedCommit, []capabilityGenerationManifestFixture{
		{version: "1.13.19", support: capability.SupportManualJSON},
	})
	if blockedDigest != firstDigest {
		t.Fatalf("identical manifest digest = %s, want %s", blockedDigest, firstDigest)
	}
	if _, err := database.SaveCapabilityGeneration(ctx, blockedSource, time.Now()); !errors.Is(err, ErrCapabilityManifestQuarantined) {
		t.Fatalf("generation containing quarantined manifest error = %v", err)
	}
	if _, err := database.CapabilityGenerationByCommit(ctx, blockedCommit); !errors.Is(err, ErrCapabilityGenerationNotFound) {
		t.Fatalf("quarantined generation became partially visible: %v", err)
	}
}

func TestCapabilityGenerationInvalidSourcePublishesNothing(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	if _, err := database.SaveCapabilityGeneration(
		ctx,
		[]byte(`{"schema_version":1,"repository":"rehuony/sing-box-panel","commit_sha":"main","manifests":[]}`),
		time.Now(),
	); !errors.Is(err, capability.ErrInvalidGeneration) {
		t.Fatalf("invalid generation error = %v", err)
	}
	generations, err := database.ListCapabilityGenerations(ctx, 20)
	if err != nil || len(generations) != 0 {
		t.Fatalf("invalid generation became visible: %+v, %v", generations, err)
	}
}

type capabilityGenerationManifestFixture struct {
	version string
	support capability.SupportLevel
}

func capabilityGenerationFixture(
	t *testing.T,
	commit string,
	fixtures []capabilityGenerationManifestFixture,
) ([]byte, string) {
	t.Helper()
	type entry struct {
		Path           string          `json:"path"`
		ManifestSHA256 string          `json:"manifest_sha256"`
		Manifest       json.RawMessage `json:"manifest"`
	}
	type envelope struct {
		SchemaVersion int     `json:"schema_version"`
		Repository    string  `json:"repository"`
		CommitSHA     string  `json:"commit_sha"`
		ManifestCount int     `json:"manifest_count"`
		Manifests     []entry `json:"manifests"`
	}
	value := envelope{
		SchemaVersion: capability.GenerationSchemaVersion,
		Repository:    capability.ManifestRepository,
		CommitSHA:     commit,
		ManifestCount: len(fixtures),
		Manifests:     make([]entry, len(fixtures)),
	}
	firstDigest := ""
	for index, fixture := range fixtures {
		version, err := coreartifact.ParseExactVersion(fixture.version)
		if err != nil {
			t.Fatal(err)
		}
		spec := capability.ManifestSpec{
			SchemaVersion: capability.ManifestSchemaVersion,
			CoreVersion:   version,
			SupportLevel:  fixture.support,
		}
		if fixture.support == capability.SupportNativeStructured ||
			fixture.support == capability.SupportCompatibleStructured {
			spec.SemanticFacts = []capability.SemanticFact{{
				ID: "global.mode", CanonicalPath: "/global/mode",
				Classification: capability.CoverageSupported,
				OwnedPaths:     []string{"/route/final"},
			}}
			spec.Transforms = []capability.Transform{{
				ID: "global.mode.rename", FactID: "global.mode", Primitive: capability.PrimitiveRename,
				From: []string{"/global/mode"}, To: []string{"/route/final"},
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
		if index == 0 {
			firstDigest = digest.String()
		}
		value.Manifests[index] = entry{
			Path: "capabilities/" + fixture.version + ".json", ManifestSHA256: digest.String(), Manifest: canonical,
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, firstDigest
}
