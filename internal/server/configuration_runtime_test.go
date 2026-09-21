// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type configurationCheckManager struct {
	fakeRuntimeManager
	checkError error
	checkHook  func()
}

func (manager *configurationCheckManager) Check(ctx context.Context, bundle coreruntime.AppliedBundle) error {
	_ = manager.fakeRuntimeManager.Check(ctx, bundle)
	if manager.checkHook != nil {
		manager.checkHook()
	}
	return manager.checkError
}

func TestConfigurationRuntimePreflightPreservesProcessAndFencesChanges(t *testing.T) {
	for _, scenario := range []string{"success", "binary-rejected", "edited-during-check", "superseded-during-check"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db, commands, observation := seedRuntimeObservation(t, ctx)
			file, err := commands.ConfigurationFile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"},"future":9007199254740993}`})
			if err != nil {
				t.Fatal(err)
			}
			before, err := db.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			queued, err := commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentRestart)
			if err != nil {
				t.Fatal(err)
			}
			reserved, err := db.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if reserved.Hub.DesiredBundleID != before.Hub.DesiredBundleID || reserved.Hub.AppliedBundleID != before.Hub.AppliedBundleID || reserved.Hub.DesiredRunning != before.Hub.DesiredRunning {
				t.Fatal("preflight changed the desired process before validation")
			}
			manager := &configurationCheckManager{}
			if scenario == "binary-rejected" {
				manager.checkError = errors.New("binary rejected configuration")
			}
			if scenario == "edited-during-check" {
				manager.checkHook = func() {
					if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: saved.Revision, Content: "{"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "superseded-during-check" {
				manager.checkHook = func() {
					if _, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, ""); err != nil {
						t.Fatal(err)
					}
				}
			}
			services := &runtimeServices{database: db, commands: commands, manager: manager}
			bound, err := services.checkConfigurationForIntent(ctx, queued, successfulRuntimeGuard{})
			if scenario == "success" {
				if err != nil || bound.ActivationBundleID == "" {
					t.Fatalf("bind: %+v %v", bound, err)
				}
				material, err := commands.LoadRuntimeMaterial(ctx, bound.ActivationBundleID)
				if err != nil {
					t.Fatal(err)
				}
				if material.Startup.CanonicalRevisionID != saved.CanonicalRevisionID || !strings.Contains(string(manager.checkedBundle.StartupConfig), "9007199254740993") {
					t.Fatal("checked bytes did not match saved configuration")
				}
			} else if err == nil {
				t.Fatal("unsafe preflight was accepted")
			}
			current, err := db.RuntimeObservation(ctx)
			if err != nil || current != observation || manager.stopCalls != 0 {
				t.Fatalf("preflight changed the live process: %+v %v", current, err)
			}
		})
	}
}

func TestInvalidSavedConfigurationCannotStartOrRestartFromOldHistory(t *testing.T) {
	ctx := context.Background()
	db, commands, observation := seedRuntimeObservation(t, ctx)
	file, err := commands.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: "{"}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []store.RuntimeIntentKind{store.RuntimeIntentStart, store.RuntimeIntentRestart} {
		if _, err := commands.PrepareConfigurationRuntime(ctx, "", kind); !errors.Is(err, store.ErrConfigurationFileUnparsed) {
			t.Fatalf("invalid file used old history: %v", err)
		}
	}
	current, err := db.RuntimeObservation(ctx)
	if err != nil || current != observation {
		t.Fatalf("changed observation: %+v %v", current, err)
	}
}

type configurationLaunchManager struct {
	configurationCheckManager
	launches int
}

func (manager *configurationLaunchManager) Start(_ context.Context, bundle coreruntime.AppliedBundle) error {
	manager.launches++
	manager.live = coreruntime.LiveIdentity{
		Running: true, State: coreruntime.StateRunning, PID: 5151,
		BundleID: bundle.ID, ArtifactID: bundle.ArtifactID, ExactVersion: bundle.ExactVersion,
		ArtifactDigest: bundle.ArtifactDigest, StartedAt: time.Now().UTC(),
	}
	return nil
}

func (manager *configurationLaunchManager) Restart(ctx context.Context, bundle coreruntime.AppliedBundle) error {
	return manager.Start(ctx, bundle)
}

func TestSavedConfigurationRestartCommitsCheckedBytesAndLoadedIdentity(t *testing.T) {
	ctx := context.Background()
	db, commands, previous := seedRuntimeObservation(t, ctx)
	file, err := commands.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"}}`})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentRestart)
	if err != nil {
		t.Fatal(err)
	}
	manager := &configurationLaunchManager{}
	manager.live = coreruntime.LiveIdentity{Running: true, PID: previous.PID, BundleID: previous.ActivationBundleID}
	resolver := &fakeRuntimeIdentityResolver{startToken: "new-incarnation"}
	services := &runtimeServices{database: db, commands: commands, manager: manager, identity: resolver}
	if err := services.executeIntent(ctx, queued); err != nil {
		t.Fatal(err)
	}
	if manager.launches != 1 || manager.checkedBundle == nil {
		t.Fatal("incomplete lifecycle")
	}
	observation, err := db.RuntimeObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	resolver.identity = application.RuntimeIdentity{PID: observation.PID, ProcessStartToken: observation.ProcessStartToken, ActivationBundleID: observation.ActivationBundleID}
	status, err := application.FromStoreWithRuntimeResolver(db, resolver).RuntimeStatus(ctx)
	if err != nil || status.LoadedCanonicalRevisionID != saved.CanonicalRevisionID || status.EnabledCore == nil || status.EnabledCore.CoreArtifactID != observation.CoreArtifactID {
		t.Fatalf("loaded identity: %+v %v", status, err)
	}
	file, err = commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: saved.Revision, Content: `{"log":{"level":"trace"}}`})
	if err != nil {
		t.Fatal(err)
	}
	status, err = application.FromStoreWithRuntimeResolver(db, resolver).RuntimeStatus(ctx)
	if err != nil || status.LoadedCanonicalRevisionID == file.CanonicalRevisionID || status.LoadedCanonicalRevisionID != saved.CanonicalRevisionID {
		t.Fatalf("save changed loaded identity: %+v %v", status, err)
	}
}

func TestStartDoesNotReloadAnAlreadyRunningProcess(t *testing.T) {
	ctx := context.Background()
	db, commands, previous := seedRuntimeObservation(t, ctx)
	unchanged, err := commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentStart)
	if err != nil || unchanged.ActivationBundleID != previous.ActivationBundleID || unchanged.StartupArtifactID != "" {
		t.Fatalf("unchanged start: %+v %v", unchanged, err)
	}
	file, err := commands.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"}}`}); err != nil {
		t.Fatal(err)
	}
	intent, err := commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentStart)
	if err != nil {
		t.Fatal(err)
	}
	manager := &configurationLaunchManager{}
	manager.live = coreruntime.LiveIdentity{Running: true, PID: previous.PID, BundleID: previous.ActivationBundleID}
	services := &runtimeServices{database: db, commands: commands, manager: manager}
	if err := services.executeIntent(ctx, intent); err == nil {
		t.Fatal("start silently reloaded a changed configuration")
	}
	if manager.launches != 0 || manager.stopCalls != 0 {
		t.Fatal("start changed running process")
	}
}

func TestStoppedCoreSelectionCommitsWithoutLaunchingAndRetainsVersion(t *testing.T) {
	for _, scenario := range []string{"success", "binary-rejected", "canceled", "superseded"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db, commands, previous := seedRuntimeObservation(t, ctx)
			if _, err := db.ClearRuntimeObservation(ctx, previous.PID, previous.ProcessStartToken); err != nil {
				t.Fatal(err)
			}
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			artifacts, err := artifactstore.New(artifactstore.Options{Root: filepath.Join(root, "artifacts")})
			if err != nil {
				t.Fatal(err)
			}
			core, err := db.GetCoreArtifact(ctx, previous.CoreArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			core.ID = "selected-core"
			core.ArchiveSHA256 = strings.Repeat("c", 64)
			core.BinarySHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("selected binary")))
			core.BinaryPath = filepath.Join(root, "artifacts", "sha256", "cc", core.ArchiveSHA256, "sing-box")
			if err := os.MkdirAll(filepath.Dir(core.BinaryPath), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(core.BinaryPath, []byte("selected binary"), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := db.UpsertCoreArtifact(ctx, core); err != nil {
				t.Fatal(err)
			}
			file, err := commands.ConfigurationFile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := commands.SaveConfigurationFile(ctx, application.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"debug"}}`})
			if err != nil {
				t.Fatal(err)
			}
			queued, err := db.RequestConfigurationRuntimeIntent(ctx, store.RuntimeIntentInput{Kind: store.RuntimeIntentRestart, SelectOnly: true, CreatedAt: time.Now().UTC()}, store.StartupArtifact{
				ID: "selected-startup", CanonicalRevisionID: saved.CanonicalRevisionID, ExactCoreVersion: previous.ExactCoreVersion,
				CoreArtifactID: core.ID, ConfigBytes: []byte(saved.Content), CreatedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			manager := &configurationLaunchManager{}
			if scenario == "binary-rejected" {
				manager.checkError = errors.New("invalid configuration")
			}
			resolver := &fakeRuntimeIdentityResolver{err: application.ErrNoRunningCore}
			services := &runtimeServices{database: db, commands: commands, manager: manager, identity: resolver}
			actionContext, cancel := context.WithCancel(ctx)
			defer cancel()
			if scenario == "canceled" {
				manager.checkHook = cancel
			}
			if scenario == "superseded" {
				manager.checkHook = func() {
					if _, err := commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, ""); err != nil {
						t.Fatal(err)
					}
				}
			}
			handleErr := services.executeIntent(actionContext, queued)
			if scenario == "success" && handleErr != nil {
				t.Fatal(handleErr)
			}
			if scenario != "success" && handleErr == nil {
				t.Fatal("unsafe selection accepted")
			}
			if manager.launches != 0 || manager.stopCalls != 0 {
				t.Fatal("selection changed process state")
			}
			bootstrap, err := db.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "success" {
				if bootstrap.Hub.DesiredRunning || bootstrap.Hub.AppliedBundleID == previous.ActivationBundleID {
					t.Fatalf("selection not committed: %+v", bootstrap.Hub)
				}
				status, err := application.FromStoreWithRuntimeResolver(db, resolver).RuntimeStatus(ctx)
				if err != nil || status.EnabledCore == nil || status.EnabledCore.CoreArtifactID != core.ID || status.Running != nil || status.LoadedCanonicalRevisionID != "" {
					t.Fatalf("stopped selection: %+v %v", status, err)
				}
				if err := commands.SyncEnabledCoreLink(ctx, artifacts); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(root, "artifacts", "current")
				if target, err := filepath.EvalSymlinks(link); err != nil || target != core.BinaryPath {
					t.Fatalf("selection link: %q %v", target, err)
				}
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := application.FromStore(db).SyncEnabledCoreLink(ctx, artifacts); err != nil {
					t.Fatal(err)
				}
				if target, err := filepath.EvalSymlinks(link); err != nil || target != core.BinaryPath {
					t.Fatalf("recovered link: %q %v", target, err)
				}
				start, err := commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentStart)
				if err != nil || start.ActivationBundleID != bootstrap.Hub.AppliedBundleID {
					t.Fatalf("start lost selection: %+v %v", start, err)
				}
			} else if bootstrap.Hub.AppliedBundleID != previous.ActivationBundleID {
				t.Fatal("unsuccessful operation changed selection")
			}
		})
	}
}
