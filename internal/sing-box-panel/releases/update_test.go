// SPDX-License-Identifier: GPL-3.0-or-later

package releases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

// A system service hands its update to the root helper; the panel follows
// the helper's result while it runs.
func TestStagedUpdateFollowsTheHelper(t *testing.T) {
	now := time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)
	staged := func(t *testing.T) (*Service, string) {
		t.Helper()
		service := New(buildinfo.Info{Version: "v1.0.0"}, t.TempDir(), nil, nil, Options{Now: func() time.Time { return now }})
		service.update = updateState{state: StateStaged, target: "v1.1.0"}
		updates := service.updatesDir()
		if err := os.MkdirAll(updates, 0o700); err != nil {
			t.Fatal(err)
		}
		return service, updates
	}
	writeResult := func(t *testing.T, updates string, attempt selfupdate.Attempt) {
		t.Helper()
		data, err := json.Marshal(attempt)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(updates, "result"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("installed", func(t *testing.T) {
		service, updates := staged(t)
		writeResult(t, updates, selfupdate.Attempt{Version: "v1.1.0", Succeeded: true, FinishedAt: now})
		service.takeHelperResult()
		status := service.Status()
		if status.State != StateRestarting || status.TargetVersion == nil || *status.TargetVersion != "v1.1.0" ||
			status.LastAttempt == nil || !status.LastAttempt.Succeeded {
			t.Fatalf("status after the helper installed the release = %+v", status)
		}
	})

	t.Run("refused", func(t *testing.T) {
		service, updates := staged(t)
		writeResult(t, updates, selfupdate.Attempt{Version: "v1.1.0", Code: "release_invalid", FinishedAt: now})
		service.takeHelperResult()
		status := service.Status()
		if status.State != StateIdle || status.TargetVersion != nil || status.LastAttempt == nil || status.LastAttempt.Code != "release_invalid" {
			t.Fatalf("status after the helper refused the release = %+v", status)
		}
		// The result counts once.
		service.update.state = StateStaged
		service.takeHelperResult()
		if service.Status().State != StateStaged {
			t.Fatal("a result was taken twice")
		}
	})

	t.Run("unclaimed", func(t *testing.T) {
		service, updates := staged(t)
		request := filepath.Join(updates, selfupdate.RequestName)
		if err := os.WriteFile(request, []byte("v1.1.0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(request, now, now); err != nil {
			t.Fatal(err)
		}
		service.takeHelperResult()
		if service.Status().State != StateStaged {
			t.Fatal("a fresh request was withdrawn")
		}
		old := now.Add(-selfupdate.HelperTimeout - time.Second)
		if err := os.Chtimes(request, old, old); err != nil {
			t.Fatal(err)
		}
		service.takeHelperResult()
		status := service.Status()
		if status.State != StateIdle || status.LastAttempt == nil || status.LastAttempt.Code != selfupdate.FailureCode(selfupdate.ErrHelperUnavailable) {
			t.Fatalf("status after the helper never took the release = %+v", status)
		}
		if _, err := os.Lstat(request); !os.IsNotExist(err) {
			t.Fatalf("withdrawn request = %v", err)
		}
	})
}

func TestUpdateRefusesWithoutAKnownNewerRelease(t *testing.T) {
	service := New(buildinfo.Info{Version: "v1.0.0"}, t.TempDir(), nil, nil, Options{})
	if !service.supported() {
		if _, err := service.Update(t.Context()); err != ErrUpdateUnsupported {
			t.Fatalf("update on an unsupported platform = %v", err)
		}
		return
	}
	if _, err := service.Update(t.Context()); err != ErrNoUpdate {
		t.Fatalf("update without a checked release = %v", err)
	}
	service.update.latest = "v1.0.0"
	if _, err := service.Update(t.Context()); err != ErrNoUpdate {
		t.Fatalf("update to the running version = %v", err)
	}
	service.update.state = StateRestartRequired
	service.update.latest = "v1.1.0"
	if _, err := service.Update(t.Context()); err != ErrUpdateRunning {
		t.Fatalf("update while one awaits its restart = %v", err)
	}
}
