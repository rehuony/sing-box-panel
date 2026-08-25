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
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestManualAndRuntimeCommandsUseExactDurableArtifacts(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	initialOutput := runApplicationCommand(t, settingsPath,
		`{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}`,
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
		ID: "core-cli", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "amd64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceOfficial, RepositoryID: 1, ReleaseID: 2, AssetID: 3,
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/secure/core-cli/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"features":[]}`), VerificationState: store.CoreArtifactVerified,
		CreatedAt: time.Date(2026, time.August, 26, 20, 0, 0, 0, time.UTC),
	}
	if _, err := database.UpsertCoreArtifact(context.Background(), core); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	manualBytes := "{\n // preserved\n \"log\": {},\n}\n"
	previewOutput := runApplicationCommand(t, settingsPath, manualBytes,
		"--output", "json", "config", "manual", "preview", "--file", "-",
		"--base-revision", initial.Revision.ID, "--core-version", "1.13.19",
		"--artifact", core.ID,
	)
	var preview application.ManualReplacePreview
	if err := json.Unmarshal(previewOutput, &preview); err != nil ||
		preview.Resolution.Source != "explicit" || preview.Reverse.Available {
		t.Fatalf("manual preview=%+v err=%v output=%s", preview, err, previewOutput)
	}

	manualOutput := runApplicationCommand(t, settingsPath, manualBytes,
		"--output", "json", "config", "manual", "replace", "--file", "-",
		"--base-revision", initial.Revision.ID, "--core-version", "1.13.19",
		"--artifact", core.ID, "--detach",
	)
	var manual application.ManualSave
	if err := json.Unmarshal(manualOutput, &manual); err != nil {
		t.Fatalf("decode manual output: %v; %s", err, manualOutput)
	}
	if manual.Artifact.CoreArtifactID != core.ID || manual.Artifact.ExactCoreVersion != "1.13.19" ||
		manual.Task.Kind != "startup-check" {
		t.Fatalf("manual save = %+v", manual)
	}

	checkOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "core", "check", manual.Artifact.ID, "--detach",
	)
	var check application.Task
	if err := json.Unmarshal(checkOutput, &check); err != nil || check.ID != manual.Task.ID {
		t.Fatalf("check=%+v err=%v output=%s", check, err, checkOutput)
	}

	database, err = store.Open(context.Background(), filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CompleteStartupArtifactCheck(
		context.Background(), manual.Artifact.ID, true,
		json.RawMessage(`[{"code":"fixture_checked"}]`), time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	applyOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "config", "apply", "--artifact", manual.Artifact.ID, "--detach",
	)
	var apply struct {
		Activation application.ActivationSummary `json:"activation"`
		Task       application.Task              `json:"task"`
	}
	if err := json.Unmarshal(applyOutput, &apply); err != nil {
		t.Fatalf("decode apply: %v; %s", err, applyOutput)
	}
	if apply.Activation.StartupArtifactID != manual.Artifact.ID ||
		apply.Task.Kind != string(store.RuntimeIntentApply) || apply.Task.ActivationBundleID != apply.Activation.ActivationBundleID {
		t.Fatalf("apply = %+v", apply)
	}
}

func TestManualReplaceWithoutCoreVersionDoesNotSelectLatest(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(`{"log":{}}`), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{
		"--config", settingsPath, "config", "manual", "replace", "--file", "-",
		"--base-revision", "rev-does-not-matter", "--artifact", "core-does-not-matter", "--detach",
	})
	err := command.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "currently running") {
		t.Fatalf("omitted version error=%v exit=%d", err, ExitCode(err))
	}
}
