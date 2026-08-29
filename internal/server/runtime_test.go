// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestReconcileStartupClearsOnlyProvenStaleObservation(t *testing.T) {
	tests := []struct {
		name        string
		identity    application.RuntimeIdentity
		identityErr error
		wantErr     error
		wantCleared bool
	}{
		{
			name:        "stale observation",
			identityErr: fmtError(application.ErrStaleObservation, "process incarnation ended"),
			wantCleared: true,
		},
		{
			name:        "inspection unavailable",
			identityErr: application.ErrInspectionUnavailable,
			wantErr:     application.ErrInspectionUnavailable,
		},
		{
			name:        "unknown resolver failure",
			identityErr: errors.New("identity database unavailable"),
		},
		{
			name:     "confirmed live process",
			identity: application.RuntimeIdentity{PID: 4242, ActivationBundleID: "bundle-runtime-test"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database, commands, observation := seedRuntimeObservation(t, ctx)
			resolver := &fakeRuntimeIdentityResolver{identity: test.identity, err: test.identityErr}
			services := &runtimeServices{
				database: database, commands: commands, manager: &fakeRuntimeManager{}, identity: resolver,
			}
			err := services.ReconcileStartup(ctx)
			switch {
			case test.wantCleared:
				if err != nil {
					t.Fatalf("ReconcileStartup() error = %v", err)
				}
				if _, err := database.RuntimeObservation(ctx); !errors.Is(err, store.ErrRuntimeObservationNotFound) {
					t.Fatalf("stale observation was not cleared: %v", err)
				}
			case test.wantErr != nil:
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ReconcileStartup() error = %v, want %v", err, test.wantErr)
				}
			default:
				if err == nil {
					t.Fatal("ReconcileStartup() unexpectedly succeeded")
				}
			}
			if !test.wantCleared {
				stored, observationErr := database.RuntimeObservation(ctx)
				if observationErr != nil || stored.PID != observation.PID || stored.ProcessStartToken != observation.ProcessStartToken {
					t.Fatalf("ambiguous observation was changed: observation=%+v err=%v", stored, observationErr)
				}
			}
			if resolver.resolveCalls != 1 {
				t.Fatalf("Resolve() calls = %d, want 1", resolver.resolveCalls)
			}
		})
	}
}

func TestReconcileStartupFailsClosedOnObservationReadError(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	resolver := &fakeRuntimeIdentityResolver{err: application.ErrStaleObservation}
	services := &runtimeServices{
		database: database, commands: commands, manager: &fakeRuntimeManager{}, identity: resolver,
	}
	if err := services.ReconcileStartup(ctx); err == nil {
		t.Fatal("ReconcileStartup() succeeded after the database was closed")
	}
	if resolver.resolveCalls != 0 {
		t.Fatalf("Resolve() calls = %d, want 0", resolver.resolveCalls)
	}
}

func TestStopForTaskFailsBeforeStoppingProcessOnObservationReadError(t *testing.T) {
	ctx := context.Background()
	database, commands, _ := seedRuntimeObservation(t, ctx)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	manager := &fakeRuntimeManager{}
	services := &runtimeServices{database: database, commands: commands, manager: manager}
	if _, err := services.stopForTask(ctx, successfulTaskControl{}); err == nil {
		t.Fatal("stopForTask() succeeded after observation read failed")
	}
	if manager.stopCalls != 0 {
		t.Fatalf("manager.Stop() calls = %d, want 0", manager.stopCalls)
	}
}

func seedRuntimeObservation(
	t *testing.T,
	ctx context.Context,
) (*store.Store, *application.Application, store.RuntimeObservation) {
	t.Helper()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", store.NewCanonicalRevision{
		ID: "revision-runtime-test", SchemaVersion: configuration.SchemaVersion,
		Document: configuration.Empty().CanonicalJSON(), CommandID: "command-runtime-test", CreatedAt: now,
	}, store.NewTask{
		ID: "task-runtime-canonical", IdempotencyKey: "canonical:runtime-test",
		Lane: store.TaskLaneMaintenance, Kind: store.TaskKindCanonicalSaved,
		Payload: json.RawMessage(`{}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	core := store.CoreArtifact{
		ID: "core-runtime-test", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "runtime state test",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: "/opt/sing-box-panel/core-runtime-test/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{}`), VerificationState: store.CoreArtifactVerified, CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-runtime-test", CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		AdapterID: "test-adapter", AdapterRevision: "1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{}`), Diagnostics: json.RawMessage(`[]`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, json.RawMessage(`[]`), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := database.SaveActivationBundle(ctx, store.ActivationBundle{
		ID: "bundle-runtime-test", StartupArtifactID: startup.ID,
		MonitoringTier: store.MonitoringProcessOnly, CreatedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := database.RecordRuntimeObservation(ctx, store.RuntimeObservation{
		PID: 4242, ProcessStartToken: "process-start-runtime-test", CoreArtifactID: core.ID,
		ActivationBundleID: bundle.ID, ExactCoreVersion: core.ExactVersion,
		ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256,
		StartedAt: now.Add(3 * time.Second), ObservedAt: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return database, application.FromStore(database), observation
}

func fmtError(kind error, detail string) error {
	return errors.Join(kind, errors.New(detail))
}

type fakeRuntimeIdentityResolver struct {
	identity     application.RuntimeIdentity
	err          error
	resolveCalls int
}

func (resolver *fakeRuntimeIdentityResolver) Resolve(context.Context) (application.RuntimeIdentity, error) {
	resolver.resolveCalls++
	return resolver.identity, resolver.err
}

func (*fakeRuntimeIdentityResolver) ProcessStartToken(context.Context, int) (string, error) {
	return "fake-process-start-token", nil
}

type fakeRuntimeManager struct {
	stopCalls int
}

func (*fakeRuntimeManager) Check(context.Context, coreruntime.AppliedBundle) error { return nil }
func (*fakeRuntimeManager) Start(context.Context, coreruntime.AppliedBundle) error { return nil }
func (manager *fakeRuntimeManager) Stop(context.Context) error {
	manager.stopCalls++
	return nil
}
func (*fakeRuntimeManager) Restart(context.Context, coreruntime.AppliedBundle) error { return nil }
func (*fakeRuntimeManager) Close(context.Context) error                              { return nil }
func (*fakeRuntimeManager) Wait() error                                              { return nil }
func (*fakeRuntimeManager) MonitoringLevel() coreruntime.MonitoringLevel {
	return coreruntime.MonitoringProcessOnly
}
func (*fakeRuntimeManager) ObserveLiveIdentity() coreruntime.LiveIdentity {
	return coreruntime.LiveIdentity{}
}

type successfulTaskControl struct{}

func (successfulTaskControl) SafePoint(context.Context) error { return nil }
