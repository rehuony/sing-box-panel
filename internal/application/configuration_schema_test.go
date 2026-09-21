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

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/singbox"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestPreviewAndCompileUseRawRevisionWithoutSchema(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := FromStore(database)
	application.SetRuntimeController(runtimeControllerFunc(func(ctx context.Context, request RuntimeRequest) (RuntimeResponse, error) {
		if request.Action != "check" {
			t.Fatalf("unexpected action %s", request.Action)
		}
		checked, err := application.CompleteStartupCheck(ctx, request.StartupArtifactID, true)
		summary := StartupArtifactSummary{ID: checked.ID, State: checked.State, CoreArtifactID: checked.CoreArtifactID}
		return RuntimeResponse{Startup: &summary}, err
	}))
	now := time.Date(2026, 8, 28, 1, 2, 3, 0, time.UTC)
	application.now = func() time.Time { return now }

	_, err = database.UpsertCoreArtifact(ctx, store.CoreArtifact{
		ID: "core_11319", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	document, err := configuration.Parse([]byte(`{"future_option":{"enabled":true},"log":{"level":"warn"}}`))
	if err != nil {
		t.Fatal(err)
	}
	revision, err := testutil.SaveConfiguration(ctx, database, 0, store.NewCanonicalRevision{
		ID: "rev_1", SchemaVersion: configuration.SchemaVersion, Document: document.CanonicalJSON(), CommandID: "cmd_1", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	preview, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11319"})
	if err != nil {
		t.Fatalf("PreviewConfiguration() error = %v", err)
	}
	if preview.Support.Structured || preview.Support.ExactVersion != "1.13.19" || preview.Support.Reason == "" {
		t.Fatalf("support = %+v", preview.Support)
	}
	if string(preview.Config) != string(revision.Document) {
		t.Fatalf("config = %s, want revision %s", preview.Config, revision.Document)
	}
	if _, err := application.ConfigurationSchema(ctx, "core_11319"); !errors.Is(err, singbox.ErrConfigurationSchemaUnavailable) {
		t.Fatalf("ConfigurationSchema() error = %v, want ErrSchemaUnavailable", err)
	}

	compiled, err := application.CompileConfiguration(ctx, ConfigurationCompileRequest{CoreArtifactID: "core_11319"})
	if err != nil {
		t.Fatalf("CompileConfiguration() error = %v", err)
	}
	if compiled.Support.Structured || compiled.Artifact.CanonicalRevisionID != revision.ID ||
		compiled.Artifact.CoreArtifactID != "core_11319" || compiled.Artifact.State != store.StartupArtifactReady {
		t.Fatalf("compile = %+v", compiled)
	}
	startup, err := database.GetStartupArtifact(ctx, compiled.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(startup.ConfigBytes) != string(revision.Document) || startup.ConfigSHA256 != revision.SHA256 {
		t.Fatalf("startup = %+v config=%s", startup, startup.ConfigBytes)
	}
	draft, err := application.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: 1, Content: "{"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11319"}); !errors.Is(err, store.ErrConfigurationFileUnparsed) {
		t.Fatalf("invalid draft fell back to prior snapshot: %v", err)
	}
	current, err := application.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: draft.Revision, Content: `{"log":{"level":"debug"}}`})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11319"})
	if err != nil || preview.CanonicalRevision.ID != current.CanonicalRevisionID || string(preview.Config) != current.Content {
		t.Fatalf("preview did not use current saved file: %+v %v", preview, err)
	}

}

func TestCompileUsesNativeSchemaByExactVersionBeforeBinaryCheck(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := FromStore(database)
	application.SetRuntimeController(runtimeControllerFunc(func(ctx context.Context, request RuntimeRequest) (RuntimeResponse, error) {
		if request.Action != "check" {
			t.Fatalf("unexpected action %s", request.Action)
		}
		checked, err := application.CompleteStartupCheck(ctx, request.StartupArtifactID, true)
		summary := StartupArtifactSummary{ID: checked.ID, State: checked.State, CoreArtifactID: checked.CoreArtifactID}
		return RuntimeResponse{Startup: &summary}, err
	}))
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	application.now = func() time.Time { return now }
	_, err = database.UpsertCoreArtifact(ctx, store.CoreArtifact{
		ID: "core_1140", ExactVersion: "1.14.0", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("c", 64),
		BinarySHA256: strings.Repeat("d", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.14.0",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testutil.SaveConfiguration(ctx, database, 0, store.NewCanonicalRevision{
		ID: "rev_invalid_1140", SchemaVersion: configuration.SchemaVersion,
		Document: json.RawMessage(`{"inbounds":"not-an-array"}`), CommandID: "cmd_invalid_1140", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	support, err := application.ConfigurationSupport(ctx, "core_1140")
	if err != nil || !support.Structured || support.ExactVersion != "1.14.0" {
		t.Fatalf("ConfigurationSupport() = %+v, %v", support, err)
	}
	_, err = application.CompileConfiguration(ctx, ConfigurationCompileRequest{CoreArtifactID: "core_1140"})
	if !errors.Is(err, ErrConfigurationSchemaValidation) {
		t.Fatalf("CompileConfiguration() error = %v, want schema validation failure", err)
	}
	artifacts, err := database.ListStartupArtifacts(ctx, store.StartupArtifactListFilter{})
	if err != nil || len(artifacts.Items) != 0 {
		t.Fatalf("startup artifacts after validation failure = %+v, %v", artifacts, err)
	}
}

type runtimeControllerFunc func(context.Context, RuntimeRequest) (RuntimeResponse, error)

func (f runtimeControllerFunc) ExecuteRuntime(ctx context.Context, r RuntimeRequest) (RuntimeResponse, error) {
	return f(ctx, r)
}
