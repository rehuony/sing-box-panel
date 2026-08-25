// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestBuildGenerationCanonicalizesAndSortsExactVersions(t *testing.T) {
	commit := strings.Repeat("a", 40)
	generation, err := BuildGeneration(commit, []ManifestFile{
		{Name: "1.10.0.json", Data: packManifestJSON(t, "1.10.0", true)},
		{Name: "1.2.9.json", Data: packManifestJSON(t, "1.2.9", false)},
	})
	if err != nil {
		t.Fatalf("BuildGeneration: %v", err)
	}
	entries := generation.Manifests()
	if len(entries) != 2 || entries[0].Path() != "capabilities/1.2.9.json" ||
		entries[1].Path() != "capabilities/1.10.0.json" {
		t.Fatalf("sorted entries = %+v", entries)
	}
	canonical, err := generation.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	decoded, err := DecodeGeneration(canonical)
	if err != nil {
		t.Fatalf("DecodeGeneration(canonical): %v; %s", err, canonical)
	}
	if decoded.Commit() != commit || decoded.Repository() != ManifestRepository {
		t.Fatalf("decoded generation = repository %q commit %q", decoded.Repository(), decoded.Commit())
	}
	if !json.Valid(canonical) || strings.Contains(string(canonical), "\n") {
		t.Fatalf("output is not compact canonical JSON: %q", canonical)
	}
}

func TestBuildGenerationRejectsInvalidSets(t *testing.T) {
	commit := strings.Repeat("b", 40)
	valid := packManifestJSON(t, "1.2.3", false)
	tests := []struct {
		name   string
		commit string
		files  []ManifestFile
		part   string
	}{
		{name: "invalid commit", commit: "HEAD", files: []ManifestFile{{Name: "1.2.3.json", Data: valid}}, part: "commit"},
		{name: "empty", commit: commit, part: "must not be empty"},
		{name: "noncanonical name", commit: commit, files: []ManifestFile{{Name: "v1.2.3.json", Data: valid}}, part: "must be <major>"},
		{name: "path name", commit: commit, files: []ManifestFile{{Name: "nested/1.2.3.json", Data: valid}}, part: "must be <major>"},
		{name: "version mismatch", commit: commit, files: []ManifestFile{{Name: "1.2.4.json", Data: valid}}, part: "declares core_version"},
		{name: "invalid manifest", commit: commit, files: []ManifestFile{{Name: "1.2.3.json", Data: []byte(`{"schema_version":1}`)}}, part: "invalid capability manifest"},
		{name: "duplicate version", commit: commit, files: []ManifestFile{{Name: "1.2.3.json", Data: valid}, {Name: "1.2.3.json", Data: valid}}, part: "duplicate exact core version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildGeneration(test.commit, test.files)
			if !errors.Is(err, ErrInvalidGeneration) || !strings.Contains(err.Error(), test.part) {
				t.Fatalf("BuildGeneration error = %v, want ErrInvalidGeneration containing %q", err, test.part)
			}
		})
	}
}

func packManifestJSON(t *testing.T, exactVersion string, indented bool) []byte {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  SupportManualJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !indented {
		return canonical
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, canonical, "", "  "); err != nil {
		t.Fatal(err)
	}
	return pretty.Bytes()
}
