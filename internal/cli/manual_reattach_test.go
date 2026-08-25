// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/reconcile"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestManualReattachCLIUsesStableArtifactAndDecisionDocument(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	initialOutput := runApplicationCommand(t, settingsPath,
		`{"schema_version":1,"global":{"mode":"direct"},"nodes":[],"rules":[],"subscription":{}}`,
		"--output", "json", "config", "replace", "--file", "-", "--base-revision", "none",
	)
	var initial application.CanonicalSave
	if err := json.Unmarshal(initialOutput, &initial); err != nil {
		t.Fatal(err)
	}
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	core := store.CoreArtifact{
		ID: "core-cli-reattach", ExactVersion: "1.13.19", OperatingSystem: "linux",
		Architecture: "amd64", Variant: "plain", SourceKind: store.CoreArtifactSourceOfficial,
		RepositoryID: 1, ReleaseID: 991, AssetID: 992,
		ArchiveSHA256: strings.Repeat("c", 64), BinarySHA256: strings.Repeat("d", 64),
		BinaryPath: "/secure/core-cli-reattach/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"features":[]}`),
		VerificationState:  store.CoreArtifactVerified, CreatedAt: time.Now().UTC(),
	}
	if _, err := database.UpsertCoreArtifact(context.Background(), core); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	manualOutput := runApplicationCommand(t, settingsPath,
		"{\n // exact residual\n \"route_mode\": \"reject\", \"unknown\": true\n}\n",
		"--output", "json", "config", "manual", "replace", "--file", "-",
		"--base-revision", initial.Revision.ID, "--core-version", "1.13.19",
		"--artifact", core.ID, "--detach",
	)
	var manual application.ManualSave
	if err := json.Unmarshal(manualOutput, &manual); err != nil {
		t.Fatalf("decode manual: %v; %s", err, manualOutput)
	}
	app, err := application.Open(context.Background(), settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("e", 40)
	generation, digest := cliManualReattachGeneration(t, commit)
	if _, err := app.RefreshCapabilityGeneration(context.Background(), generation); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpgradeCapability(context.Background(), application.CapabilityUpgradeRequest{
		ExactCoreVersion: "1.13.19", CommitSHA: commit, ManifestSHA256: digest,
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	previewOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "manual", "reattach", "preview", manual.Artifact.ID,
	)
	var preview application.ManualReattachPreview
	if err := json.Unmarshal(previewOutput, &preview); err != nil {
		t.Fatalf("decode preview: %v; %s", err, previewOutput)
	}
	if preview.Evidence.StartupArtifactID != manual.Artifact.ID || len(preview.Conflicts) != 0 ||
		len(preview.ResidualPaths) != 1 || preview.ResidualPaths[0] != "/unknown" {
		t.Fatalf("preview = %+v", preview)
	}
	decisionDocument, err := json.Marshal(application.ManualReattachApplyRequest{
		Evidence:  preview.Evidence,
		Decisions: map[string]reconcile.Choice{},
	})
	if err != nil {
		t.Fatal(err)
	}
	applyOutput := runApplicationCommand(t, settingsPath, string(decisionDocument),
		"--output", "json", "config", "manual", "reattach", "apply", manual.Artifact.ID,
		"--file", "-", "--detach",
	)
	var saved application.ManualReattachSave
	if err := json.Unmarshal(applyOutput, &saved); err != nil {
		t.Fatalf("decode apply: %v; %s", err, applyOutput)
	}
	if saved.Artifact.ID == manual.Artifact.ID || saved.Artifact.Raw != manual.Artifact.Raw ||
		saved.Revision.ParentID != initial.Revision.ID || saved.Task.Kind != "startup-check" {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestManualReattachCLIDecisionsRejectArgvAndUnknownJSON(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{
		"--config", settingsPath, "config", "manual", "reattach", "apply", "startup-id",
	})
	err := command.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("missing decision file error = %v", err)
	}
	if _, err := readManualReattachInput(
		strings.NewReader(`{"evidence":{},"decisions":{},"secret":"argv"}`),
		"-",
	); err == nil {
		t.Fatal("decision document accepted an unknown field")
	}
}

func cliManualReattachGeneration(t *testing.T, commit string) ([]byte, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion, CoreVersion: version,
		SupportLevel: capability.SupportNativeStructured,
		SemanticFacts: []capability.SemanticFact{{
			ID: "global.mode", CanonicalPath: "/global/mode",
			Classification: capability.CoverageSupported, OwnedPaths: []string{"/route_mode"},
		}},
		Transforms: []capability.Transform{{
			ID: "global.mode.enum", FactID: "global.mode", Primitive: capability.PrimitiveEnum,
			From: []string{"/global/mode"}, To: []string{"/route_mode"},
			Enum: map[string]string{"direct": "direct", "block": "reject"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	envelope := struct {
		SchemaVersion int    `json:"schema_version"`
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
		Repository:    capability.ManifestRepository, CommitSHA: commit, ManifestCount: 1,
	}
	envelope.Manifests = append(envelope.Manifests, struct {
		Path           string          `json:"path"`
		ManifestSHA256 string          `json:"manifest_sha256"`
		Manifest       json.RawMessage `json:"manifest"`
	}{
		Path: "capabilities/1.13.19.json", ManifestSHA256: digest.String(), Manifest: manifestJSON,
	})
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, digest.String()
}
