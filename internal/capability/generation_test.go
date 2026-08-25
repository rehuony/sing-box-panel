// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestGenerationValidatesAndCanonicalizesWholeCommit(t *testing.T) {
	t.Parallel()

	first := mustGenerationManifest(t, "1.13.19")
	second := mustGenerationManifest(t, "1.12.8")
	encoded := encodeGenerationFixture(t, strings.Repeat("a", 40), []generationFixture{first, second})
	generation, err := DecodeGeneration(encoded)
	if err != nil {
		t.Fatalf("DecodeGeneration: %v", err)
	}
	if generation.Repository() != ManifestRepository || generation.Commit() != strings.Repeat("a", 40) {
		t.Fatalf("generation identity = %s@%s", generation.Repository(), generation.Commit())
	}
	entries := generation.Manifests()
	if len(entries) != 2 || entries[0].Manifest().CoreVersion().String() != "1.12.8" ||
		entries[1].Manifest().CoreVersion().String() != "1.13.19" {
		t.Fatalf("generation entries are not normalized by exact version: %+v", entries)
	}
	firstDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	callerManifest := entries[0].Manifest()
	if err := json.Unmarshal(first.Manifest, callerManifest); err != nil {
		t.Fatal(err)
	}
	stableDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if stableDigest != firstDigest {
		t.Fatalf("caller mutation changed generation digest: %s != %s", stableDigest, firstDigest)
	}
	reordered := encodeGenerationFixture(t, strings.Repeat("a", 40), []generationFixture{second, first})
	reorderedGeneration, err := DecodeGeneration(reordered)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := reorderedGeneration.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("generation order changed digest: %s != %s", firstDigest, secondDigest)
	}
}

func TestGenerationRejectsUntrustedOrInconsistentContent(t *testing.T) {
	t.Parallel()

	valid := mustGenerationManifest(t, "1.13.19")
	tests := []struct {
		name   string
		mutate func(*generationFixtureEnvelope)
		want   string
	}{
		{name: "other repository", mutate: func(value *generationFixtureEnvelope) { value.Repository = "attacker/panel" }, want: "repository"},
		{name: "mutable commit", mutate: func(value *generationFixtureEnvelope) { value.CommitSHA = "main" }, want: "commit_sha"},
		{name: "path traversal", mutate: func(value *generationFixtureEnvelope) { value.Manifests[0].Path = "capabilities/../payload.json" }, want: "path"},
		{name: "digest mismatch", mutate: func(value *generationFixtureEnvelope) { value.Manifests[0].ManifestSHA256 = strings.Repeat("b", 64) }, want: "digest mismatch"},
		{name: "truncated index", mutate: func(value *generationFixtureEnvelope) { value.ManifestCount = 2 }, want: "manifest_count"},
		{name: "duplicate version", mutate: func(value *generationFixtureEnvelope) {
			value.Manifests = append(value.Manifests, value.Manifests[0])
			value.ManifestCount = len(value.Manifests)
		}, want: "duplicate exact"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			envelope := generationFixtureEnvelope{
				SchemaVersion: GenerationSchemaVersion,
				Repository:    ManifestRepository,
				CommitSHA:     strings.Repeat("a", 40),
				ManifestCount: 1,
				Manifests:     []generationFixture{valid},
			}
			test.mutate(&envelope)
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			_, err = DecodeGeneration(encoded)
			if !errors.Is(err, ErrInvalidGeneration) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeGeneration() error = %v, want ErrInvalidGeneration containing %q", err, test.want)
			}
		})
	}
}

func TestGenerationRejectsExecutableManifestSurface(t *testing.T) {
	t.Parallel()

	valid := mustGenerationManifest(t, "1.13.19")
	var document map[string]any
	if err := json.Unmarshal(valid.Manifest, &document); err != nil {
		t.Fatal(err)
	}
	document["$ref"] = "https://example.invalid/manifest.json"
	valid.Manifest, _ = json.Marshal(document)
	encoded := encodeGenerationFixture(t, strings.Repeat("a", 40), []generationFixture{valid})
	_, err := DecodeGeneration(encoded)
	if !errors.Is(err, ErrInvalidGeneration) || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("DecodeGeneration(remote ref) error = %v", err)
	}
}

type generationFixtureEnvelope struct {
	SchemaVersion uint32              `json:"schema_version"`
	Repository    string              `json:"repository"`
	CommitSHA     string              `json:"commit_sha"`
	ManifestCount int                 `json:"manifest_count"`
	Manifests     []generationFixture `json:"manifests"`
}

type generationFixture struct {
	Path           string          `json:"path"`
	ManifestSHA256 string          `json:"manifest_sha256"`
	Manifest       json.RawMessage `json:"manifest"`
}

func mustGenerationManifest(t *testing.T, exactVersion string) generationFixture {
	t.Helper()
	spec := enumManifestSpec(t, map[string]string{"direct": "direct", "block": "reject"})
	spec.CoreVersion = version(t, exactVersion)
	manifest, err := NewManifest(spec)
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
	return generationFixture{
		Path: "capabilities/" + exactVersion + ".json", ManifestSHA256: digest.String(), Manifest: canonical,
	}
}

func encodeGenerationFixture(t *testing.T, commit string, manifests []generationFixture) []byte {
	t.Helper()
	encoded, err := json.Marshal(generationFixtureEnvelope{
		SchemaVersion: GenerationSchemaVersion,
		Repository:    ManifestRepository,
		CommitSHA:     commit,
		ManifestCount: len(manifests),
		Manifests:     manifests,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
