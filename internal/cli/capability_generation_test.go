// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCapabilityCLIRefreshIsCandidateOnlyAndUpgradeRequiresAccept(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	commit := strings.Repeat("a", 40)
	source, digest := cliCapabilityGeneration(t, commit, "1.13.19")
	refreshOutput := runApplicationCommand(
		t,
		settingsPath,
		string(source),
		"--output", "json", "core", "capability", "refresh", "--file", "-",
	)
	var refresh application.CapabilityGenerationRefresh
	if err := json.Unmarshal(refreshOutput, &refresh); err != nil {
		t.Fatalf("decode refresh: %v; %s", err, refreshOutput)
	}
	if !refresh.Created || refresh.Generation.CommitSHA != commit || len(refresh.Candidates) != 1 {
		t.Fatalf("refresh = %+v", refresh)
	}

	statusOutput := runApplicationCommand(
		t,
		settingsPath,
		"",
		"--output", "json", "core", "capability", "status", "--core-version", "1.13.19",
	)
	var status application.CapabilityStatus
	if err := json.Unmarshal(statusOutput, &status); err != nil {
		t.Fatal(err)
	}
	if status.Pinned {
		t.Fatalf("refresh moved pin: %+v", status)
	}

	previewOutput := runApplicationCommand(
		t,
		settingsPath,
		"",
		"--output", "json", "core", "capability", "upgrade",
		"--core-version", "1.13.19", "--commit", commit, "--sha256", digest,
	)
	var preview application.CapabilityUpgradePreview
	if err := json.Unmarshal(previewOutput, &preview); err != nil {
		t.Fatalf("decode preview: %v; %s", err, previewOutput)
	}
	if !preview.Changed || preview.Blocked || preview.Candidate.ManifestSHA256 != digest {
		t.Fatalf("preview = %+v", preview)
	}
	statusOutput = runApplicationCommand(
		t,
		settingsPath,
		"",
		"--output", "json", "core", "capability", "status", "--core-version", "1.13.19",
	)
	if err := json.Unmarshal(statusOutput, &status); err != nil {
		t.Fatal(err)
	}
	if status.Pinned {
		t.Fatal("preview-only upgrade moved the pin")
	}

	upgradeOutput := runApplicationCommand(
		t,
		settingsPath,
		"",
		"--output", "json", "core", "capability", "upgrade",
		"--core-version", "1.13.19", "--commit", commit, "--sha256", digest, "--accept",
	)
	var upgrade application.CapabilityUpgrade
	if err := json.Unmarshal(upgradeOutput, &upgrade); err != nil {
		t.Fatalf("decode upgrade: %v; %s", err, upgradeOutput)
	}
	if upgrade.Pin.ManifestSHA256 != digest || upgrade.Pin.ExactCoreVersion != "1.13.19" {
		t.Fatalf("upgrade = %+v", upgrade)
	}

	inspectOutput := runApplicationCommand(
		t,
		settingsPath,
		"",
		"--output", "json", "core", "capability", "inspect",
		"--core-version", "1.13.19", "--commit", commit, "--sha256", digest,
	)
	var inspected application.CapabilityManifestCandidate
	if err := json.Unmarshal(inspectOutput, &inspected); err != nil || len(inspected.Manifest) == 0 {
		t.Fatalf("inspect = %+v, %v; %s", inspected, err, inspectOutput)
	}
	for name, output := range map[string][]byte{
		"upgrade preview": previewOutput,
		"upgrade":         upgradeOutput,
		"inspect":         inspectOutput,
	} {
		var envelope struct {
			Resolution application.CoreVersionResolution `json:"resolution"`
		}
		if err := json.Unmarshal(output, &envelope); err != nil ||
			envelope.Resolution.ExactVersion != "1.13.19" || envelope.Resolution.Source != "explicit" {
			t.Fatalf("%s resolution = %+v, err=%v; %s", name, envelope.Resolution, err, output)
		}
	}
}

func TestCapabilityCLIUpgradeWithoutVersionRequiresActualRunningCore(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	command := NewRootCommand(Dependencies{
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{
		"--config", settingsPath,
		"core", "capability", "upgrade",
		"--commit", strings.Repeat("a", 40),
		"--sha256", strings.Repeat("b", 64),
	})
	err := command.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "currently running") {
		t.Fatalf("missing running core error = %v, exit = %d", err, ExitCode(err))
	}
}

func TestCapabilityCLIQuarantineIsPermanentAndClassified(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	digest := strings.Repeat("c", 64)
	output := runApplicationCommand(
		t,
		settingsPath,
		"",
		"--output", "json", "core", "capability", "quarantine",
		"--sha256", digest, "--reason", "security_advisory",
	)
	var quarantined application.CapabilityQuarantineView
	if err := json.Unmarshal(output, &quarantined); err != nil {
		t.Fatalf("decode quarantine: %v; %s", err, output)
	}
	if quarantined.ManifestSHA256 != digest || quarantined.ReasonCode != "security_advisory" || quarantined.QuarantinedAt.IsZero() {
		t.Fatalf("quarantine = %+v", quarantined)
	}

	command := NewRootCommand(Dependencies{
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{
		"--config", settingsPath, "core", "capability", "quarantine",
		"--sha256", digest, "--reason", "operator_validation_failed",
	})
	err := command.ExecuteContext(context.Background())
	var classified *Error
	if !errors.As(err, &classified) || ExitCode(err) != 4 || classified.Code != "capability_quarantine_conflict" {
		t.Fatalf("changed reason error = %#v, exit=%d", err, ExitCode(err))
	}

	missing := NewRootCommand(Dependencies{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	missing.SetArgs([]string{"core", "capability", "quarantine", "--sha256", digest})
	err = missing.ExecuteContext(context.Background())
	if !errors.As(err, &classified) || ExitCode(err) != 2 || classified.Code != "capability_quarantine_flag_required" {
		t.Fatalf("missing reason error = %#v, exit=%d", err, ExitCode(err))
	}
}

func TestCapabilityCLIOmittedVersionUsesRunningExactVersion(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	const runningVersion = "1.12.8"
	running := runtimeidentity.Identity{
		PID: 4242, ProcessStartToken: "running-incarnation", ExactCoreVersion: runningVersion,
		CoreArtifactID: "core-running", ArchiveSHA256: strings.Repeat("c", 64),
		BinarySHA256: strings.Repeat("d", 64), ActivationBundleID: "bundle-running",
	}
	open := func(context.Context, string) (*application.Application, error) {
		return application.FromStoreWithRuntimeResolver(
			database,
			capabilityCLIRuntimeResolver{identity: running},
		), nil
	}
	commit := strings.Repeat("e", 40)
	source, digest := cliCapabilityGeneration(t, commit, runningVersion)
	runCLICommandWithOpen(
		t, open, string(source),
		"core", "capability", "refresh", "--file", "-",
	)

	inspectOutput := runCLICommandWithOpen(
		t, open, "",
		"core", "capability", "inspect", "--commit", commit, "--sha256", digest,
	)
	var inspect struct {
		Resolution application.CoreVersionResolution `json:"resolution"`
		application.CapabilityManifestCandidate
	}
	if err := json.Unmarshal(inspectOutput, &inspect); err != nil ||
		inspect.ExactCoreVersion != runningVersion || len(inspect.Manifest) == 0 {
		t.Fatalf("inspect = %+v, err=%v; %s", inspect, err, inspectOutput)
	}
	assertRunningCapabilityResolution(t, "inspect", inspect.Resolution, running)

	previewOutput := runCLICommandWithOpen(
		t, open, "",
		"core", "capability", "upgrade", "--commit", commit, "--sha256", digest,
	)
	var preview struct {
		Resolution application.CoreVersionResolution `json:"resolution"`
		application.CapabilityUpgradePreview
	}
	if err := json.Unmarshal(previewOutput, &preview); err != nil ||
		preview.Blocked || !preview.Changed || preview.Candidate.ExactCoreVersion != runningVersion {
		t.Fatalf("preview = %+v, err=%v; %s", preview, err, previewOutput)
	}
	assertRunningCapabilityResolution(t, "upgrade preview", preview.Resolution, running)

	upgradeOutput := runCLICommandWithOpen(
		t, open, "",
		"core", "capability", "upgrade", "--commit", commit, "--sha256", digest, "--accept",
	)
	var upgrade struct {
		Resolution application.CoreVersionResolution `json:"resolution"`
		application.CapabilityUpgrade
	}
	if err := json.Unmarshal(upgradeOutput, &upgrade); err != nil ||
		upgrade.Pin.ExactCoreVersion != runningVersion || upgrade.Pin.ManifestSHA256 != digest {
		t.Fatalf("upgrade = %+v, err=%v; %s", upgrade, err, upgradeOutput)
	}
	assertRunningCapabilityResolution(t, "upgrade", upgrade.Resolution, running)
}

func TestOmittedCoreVersionRuntimeFailuresAreUnavailable(t *testing.T) {
	failures := []struct {
		name string
		err  error
		code string
	}{
		{name: "no running core", err: runtimeidentity.ErrNoRunningCore, code: "no_running_core"},
		{name: "stale observation", err: runtimeidentity.ErrStaleObservation, code: "running_core_unavailable"},
		{name: "inspection unavailable", err: runtimeidentity.ErrInspectionUnavailable, code: "running_core_unavailable"},
	}
	commands := []struct {
		name  string
		stdin string
		args  []string
	}{
		{name: "capability status", args: []string{"core", "capability", "status"}},
		{name: "capability inspect", args: []string{"core", "capability", "inspect", "--commit", strings.Repeat("a", 40), "--sha256", strings.Repeat("b", 64)}},
		{name: "capability upgrade preview", args: []string{"core", "capability", "upgrade", "--commit", strings.Repeat("a", 40), "--sha256", strings.Repeat("b", 64)}},
		{name: "capability upgrade accepted", args: []string{"core", "capability", "upgrade", "--commit", strings.Repeat("a", 40), "--sha256", strings.Repeat("b", 64), "--accept"}},
		{name: "manual list", args: []string{"config", "manual", "list"}},
		{name: "manual preview", stdin: `{}`, args: []string{"config", "manual", "preview", "--file", "-", "--base-revision", "revision"}},
		{name: "manual replace", stdin: `{}`, args: []string{"config", "manual", "replace", "--file", "-", "--base-revision", "revision", "--detach"}},
	}

	for _, failure := range failures {
		failure := failure
		t.Run(failure.name, func(t *testing.T) {
			ctx := context.Background()
			database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			open := func(context.Context, string) (*application.Application, error) {
				return application.FromStoreWithRuntimeResolver(
					database,
					capabilityCLIRuntimeResolver{err: fmt.Errorf("resolve running identity: %w", failure.err)},
				), nil
			}
			for _, command := range commands {
				command := command
				t.Run(command.name, func(t *testing.T) {
					_, err := executeCLICommandWithOpen(open, command.stdin, command.args...)
					var classified *Error
					if err == nil || ExitCode(err) != 6 || !errors.As(err, &classified) || classified.Code != failure.code {
						t.Fatalf("error=%v classified=%+v exit=%d", err, classified, ExitCode(err))
					}
				})
			}
		})
	}
}

type capabilityCLIRuntimeResolver struct {
	identity runtimeidentity.Identity
	err      error
}

func (resolver capabilityCLIRuntimeResolver) Resolve(context.Context) (runtimeidentity.Identity, error) {
	return resolver.identity, resolver.err
}

func assertRunningCapabilityResolution(
	t *testing.T,
	name string,
	resolution application.CoreVersionResolution,
	want runtimeidentity.Identity,
) {
	t.Helper()
	if resolution.ExactVersion != want.ExactCoreVersion || resolution.Source != "running" ||
		resolution.Running == nil || *resolution.Running != want {
		t.Fatalf("%s resolution = %+v, want running identity %+v", name, resolution, want)
	}
}

func runCLICommandWithOpen(
	t *testing.T,
	open func(context.Context, string) (*application.Application, error),
	stdin string,
	args ...string,
) []byte {
	t.Helper()
	output, err := executeCLICommandWithOpen(open, stdin, args...)
	if err != nil {
		t.Fatalf("command %v error=%v output=%s", args, err, output)
	}
	return output
}

func executeCLICommandWithOpen(
	open func(context.Context, string) (*application.Application, error),
	stdin string,
	args ...string,
) ([]byte, error) {
	var stdout bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &bytes.Buffer{},
		OpenApplication: open,
	})
	command.SetArgs(append([]string{"--output", "json"}, args...))
	err := command.ExecuteContext(context.Background())
	return stdout.Bytes(), err
}

func cliCapabilityGeneration(t *testing.T, commit, exactVersion string) ([]byte, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  capability.SupportManualJSON,
	})
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
	value := map[string]any{
		"schema_version": capability.GenerationSchemaVersion,
		"repository":     capability.ManifestRepository,
		"commit_sha":     commit,
		"manifest_count": 1,
		"manifests": []any{map[string]any{
			"path":            "capabilities/" + exactVersion + ".json",
			"manifest_sha256": digest.String(),
			"manifest":        json.RawMessage(canonical),
		}},
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, digest.String()
}
