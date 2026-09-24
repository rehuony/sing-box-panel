// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestRuntimeConfigCleanupPreservesSQLiteEvidenceAcrossReopen(t *testing.T) {
	ctx := t.Context()
	database, _, observation := seedRuntimeObservation(t, ctx)
	core, err := database.GetCoreArtifact(ctx, observation.CoreArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	core.ID = "disposable-config-core"
	core.BinaryPath = filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(core.BinaryPath, []byte("fake executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startup, err := database.GetStartupArtifact(ctx, "startup-runtime-test")
	if err != nil {
		t.Fatal(err)
	}
	startup.ID, startup.CoreArtifactID, startup.State = "disposable-config-startup", core.ID, store.StartupArtifactPending
	startup.CheckedAt = nil
	startup, err = database.CreateStartupArtifact(ctx, startup)
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, startup.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := database.SaveActivationBundle(ctx, store.ActivationBundle{
		ID: "disposable-config-bundle", StartupArtifactID: startup.ID,
		MonitoringTier: store.MonitoringProcessOnly, CreatedAt: startup.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := database.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	path := filepath.Join(runtimeDir, "configs", startup.ConfigSHA256+".json")
	for attempt := range 2 {
		material, err := application.FromStore(database).LoadRuntimeMaterial(ctx, bundle.ID)
		if err != nil {
			t.Fatal(err)
		}
		executor := &configSnapshotExecutor{t: t, expected: startup.ConfigBytes}
		manager, err := coreruntime.NewManager(coreruntime.Options{RuntimeDir: runtimeDir, Executor: executor})
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.Check(ctx, material.Bundle); err != nil {
			t.Fatal(err)
		}
		if executor.checks != 1 {
			t.Fatal("snapshot was not reconstructed for a binary check")
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("check did not release snapshot: %v", err)
		}
		if err := manager.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if err := manager.Wait(); err != nil {
			t.Fatal(err)
		}
		if actual, err := database.GetStartupArtifact(ctx, startup.ID); err != nil || !reflect.DeepEqual(actual, startup) {
			t.Fatalf("cleanup changed startup evidence: %+v, %v", actual, err)
		}
		if actual, err := database.ConfigurationFile(ctx); err != nil || !reflect.DeepEqual(actual, saved) {
			t.Fatalf("cleanup changed saved text: %+v, %v", actual, err)
		}
		if attempt == 0 {
			databasePath := database.Path()
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			database, err = store.Open(ctx, databasePath)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
		}
	}
}

type configSnapshotExecutor struct {
	t        *testing.T
	expected []byte
	checks   int
}

func (executor *configSnapshotExecutor) Run(_ context.Context, command coreruntime.Command, _ int64) ([]byte, error) {
	if command.Args[0] == "version" {
		return []byte("sing-box version 1.13.19\n"), nil
	}
	if command.Args[0] != "check" || len(command.Args) != 3 {
		executor.t.Fatalf("unexpected command: %+v", command)
	}
	actual, err := os.ReadFile(command.Args[2])
	if err != nil || string(actual) != string(executor.expected) {
		executor.t.Fatalf("reconstructed config = %q, %v; want %q", actual, err, executor.expected)
	}
	executor.checks++
	return nil, nil
}

func (*configSnapshotExecutor) Start(coreruntime.Command) (coreruntime.ChildProcess, error) {
	return nil, errors.New("check must not start a core")
}
