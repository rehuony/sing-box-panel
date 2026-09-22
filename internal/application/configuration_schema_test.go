// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
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
		ID: "core_11318", ExactVersion: "1.13.18", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.18",
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

	preview, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11318"})
	if err != nil {
		t.Fatalf("PreviewConfiguration() error = %v", err)
	}
	if preview.Support.Structured || preview.Support.ExactVersion != "1.13.18" || preview.Support.Reason == "" {
		t.Fatalf("support = %+v", preview.Support)
	}
	if string(preview.Config) != string(revision.Document) {
		t.Fatalf("config = %s, want revision %s", preview.Config, revision.Document)
	}
	if _, err := application.ConfigurationSchema(ctx, "core_11318"); !errors.Is(err, singbox.ErrConfigurationSchemaUnavailable) {
		t.Fatalf("ConfigurationSchema() error = %v, want ErrSchemaUnavailable", err)
	}

	compiled, err := application.CompileConfiguration(ctx, ConfigurationCompileRequest{CoreArtifactID: "core_11318"})
	if err != nil {
		t.Fatalf("CompileConfiguration() error = %v", err)
	}
	if compiled.Support.Structured || compiled.Artifact.CanonicalRevisionID != revision.ID ||
		compiled.Artifact.CoreArtifactID != "core_11318" || compiled.Artifact.State != store.StartupArtifactReady {
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
	if _, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11318"}); !errors.Is(err, store.ErrConfigurationFileUnparsed) {
		t.Fatalf("invalid draft fell back to prior snapshot: %v", err)
	}
	current, err := application.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: draft.Revision, Content: `{"log":{"level":"debug"}}`})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11318"})
	if err != nil || preview.CanonicalRevision.ID != current.CanonicalRevisionID || string(preview.Config) != current.Content {
		t.Fatalf("preview did not use current saved file: %+v %v", preview, err)
	}

}

func TestCompileUsesNativeSchemaByExactVersionBeforeBinaryCheck(t *testing.T) {
	for _, exactVersion := range []string{"1.13.19", "1.13.20", "1.13.21", "1.14.0", "1.14.1"} {
		t.Run(exactVersion, func(t *testing.T) {
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
				ID: "core_native", ExactVersion: exactVersion, OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
				SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("c", 64),
				BinarySHA256: strings.Repeat("d", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: exactVersion,
				FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = testutil.SaveConfiguration(ctx, database, 0, store.NewCanonicalRevision{
				ID: "rev_invalid_native", SchemaVersion: configuration.SchemaVersion,
				Document: json.RawMessage(`{"inbounds":"not-an-array"}`), CommandID: "cmd_invalid_native", CreatedAt: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			support, err := application.ConfigurationSupport(ctx, "core_native")
			if err != nil || !support.Structured || support.ExactVersion != exactVersion {
				t.Fatalf("ConfigurationSupport() = %+v, %v", support, err)
			}
			_, err = application.CompileConfiguration(ctx, ConfigurationCompileRequest{CoreArtifactID: "core_native"})
			if !errors.Is(err, ErrConfigurationSchemaValidation) {
				t.Fatalf("CompileConfiguration() error = %v, want schema validation failure", err)
			}
			artifacts, err := database.ListStartupArtifacts(ctx, store.StartupArtifactListFilter{})
			if err != nil || len(artifacts.Items) != 0 {
				t.Fatalf("startup artifacts after validation failure = %+v, %v", artifacts, err)
			}
		})
	}
}

func TestReviewed113CompileAndRestartPreserveNullsAndDefaults(t *testing.T) {
	for _, version := range []string{"1.13.19", "1.13.20", "1.13.21"} {
		for _, fixture := range []string{"null-sections.json", "log-defaults.json", "log-null-fields.json"} {
			t.Run(version+"/"+fixture, func(t *testing.T) {
				ctx := context.Background()
				database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.Close() })
				application := FromStore(database)
				checks := 0
				application.SetRuntimeController(runtimeControllerFunc(func(ctx context.Context, request RuntimeRequest) (RuntimeResponse, error) {
					if request.Action != "check" {
						t.Fatalf("unexpected action %s", request.Action)
					}
					checks++
					checked, err := application.CompleteStartupCheck(ctx, request.StartupArtifactID, true)
					return RuntimeResponse{Startup: &StartupArtifactSummary{ID: checked.ID, State: checked.State, CoreArtifactID: checked.CoreArtifactID}}, err
				}))
				now := time.Now().UTC()
				_, err = database.UpsertCoreArtifact(ctx, store.CoreArtifact{
					ID: "core_113", ExactVersion: version, OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
					SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("c", 64),
					BinarySHA256: strings.Repeat("d", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: version,
					FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), CreatedAt: now,
				})
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join("../singbox/testdata/configuration-1.13", fixture))
				if err != nil {
					t.Fatal(err)
				}
				document, err := configuration.Parse(data)
				if err != nil {
					t.Fatal(err)
				}
				revision, err := testutil.SaveConfiguration(ctx, database, 0, store.NewCanonicalRevision{
					ID: "rev_nulls", SchemaVersion: configuration.SchemaVersion, Document: document.CanonicalJSON(),
					CommandID: "cmd_nulls", CreatedAt: now,
				})
				if err != nil {
					t.Fatal(err)
				}
				compiled, err := application.CompileConfiguration(ctx, ConfigurationCompileRequest{CoreArtifactID: "core_113"})
				if err != nil || checks != 1 {
					t.Fatalf("compile: checks=%d, error=%v", checks, err)
				}
				intent, err := application.PrepareConfigurationRuntime(ctx, "core_113", store.RuntimeIntentRestart)
				if err != nil {
					t.Fatalf("prepare restart: %v", err)
				}
				for _, id := range []string{compiled.Artifact.ID, intent.StartupArtifactID} {
					startup, err := database.GetStartupArtifact(ctx, id)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(startup.ConfigBytes, revision.Document) || startup.ConfigSHA256 != revision.SHA256 {
						t.Fatalf("startup %s changed configuration: %s", id, startup.ConfigBytes)
					}
				}
			})
		}
	}
}

type runtimeControllerFunc func(context.Context, RuntimeRequest) (RuntimeResponse, error)

func (f runtimeControllerFunc) ExecuteRuntime(ctx context.Context, r RuntimeRequest) (RuntimeResponse, error) {
	return f(ctx, r)
}
