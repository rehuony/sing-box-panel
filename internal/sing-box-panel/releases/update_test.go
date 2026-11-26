// SPDX-License-Identifier: GPL-3.0-or-later

package releases

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

// A system service's unit installs the release the panel staged before it
// starts the panel again; the panel reads the outcome when it starts.
func TestStartReadsTheInstallStepsOutcome(t *testing.T) {
	now := time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)
	start := func(t *testing.T, prepare func(updates string)) UpdateStatus {
		t.Helper()
		service := New(buildinfo.Info{Version: "v1.1.0"}, t.TempDir(), nil, nil, Options{Now: func() time.Time { return now }})
		updates := service.updatesDir()
		if err := os.MkdirAll(updates, 0o700); err != nil {
			t.Fatal(err)
		}
		prepare(updates)
		// A stopped context ends Run once it has read the outcome.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		service.Run(ctx)
		return service.Status()
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
		status := start(t, func(updates string) {
			writeResult(t, updates, selfupdate.Attempt{Version: "v1.1.0", Succeeded: true, FinishedAt: now})
		})
		if status.State != StateIdle || status.LastAttempt == nil || !status.LastAttempt.Succeeded || status.LastAttempt.Version != "v1.1.0" {
			t.Fatalf("status after the unit installed the release = %+v", status)
		}
	})

	t.Run("refused", func(t *testing.T) {
		status := start(t, func(updates string) {
			writeResult(t, updates, selfupdate.Attempt{Version: "v1.2.0", Code: "signature_invalid", FinishedAt: now})
		})
		if status.State != StateIdle || status.LastAttempt == nil || status.LastAttempt.Code != "signature_invalid" {
			t.Fatalf("status after the unit refused the release = %+v", status)
		}
	})

	t.Run("unit without the install step", func(t *testing.T) {
		var request string
		status := start(t, func(updates string) {
			request = filepath.Join(updates, selfupdate.RequestName)
			if err := os.WriteFile(request, []byte("v1.2.0\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		})
		if status.LastAttempt == nil || status.LastAttempt.Version != "v1.2.0" || status.LastAttempt.Code != "install_step_missing" {
			t.Fatalf("status after the unit left the request = %+v", status)
		}
		if _, err := os.Lstat(request); !os.IsNotExist(err) {
			t.Fatalf("withdrawn request = %v", err)
		}
	})
}

// The panel's update and node updates wait for each other: a restart of the
// panel would interrupt the nodes' updates.
func TestPanelAndNodeUpdatesWaitForEachOther(t *testing.T) {
	service := New(buildinfo.Info{Version: "v1.0.0"}, t.TempDir(), nil, nil, Options{})
	end, err := service.BeginNodeUpdate()
	if err != nil || !service.NodeUpdatesRunning() {
		t.Fatalf("begin a node update = %v", err)
	}
	if service.supported() {
		service.update.latest = "v1.1.0"
		if _, err := service.Update(t.Context()); err != ErrNodeUpdatesRunning {
			t.Fatalf("panel update while a node updates = %v", err)
		}
	}
	end()
	end()
	if service.NodeUpdatesRunning() {
		t.Fatal("an ended node update still counts")
	}
	service.update.state = StateUpdating
	if _, err := service.BeginNodeUpdate(); err != ErrUpdateRunning {
		t.Fatalf("node update while the panel updates = %v", err)
	}
}

// Any build checks for the newest release, so the settings show where it
// stands; only a release build on a supported platform is offered the update.
func TestEveryBuildChecksButOnlySupportedOnesUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.1.0","draft":false,"prerelease":false,"assets":[]}`))
	}))
	defer server.Close()
	service := New(buildinfo.Info{Version: "v1.0.0-dev"}, t.TempDir(), nil, nil, Options{HTTPClient: server.Client(), ReleasesURL: server.URL})
	status, err := service.Check(t.Context())
	if err != nil || status.LatestVersion == nil || *status.LatestVersion != "v1.1.0" || status.CheckedAt == nil {
		t.Fatalf("check of a development build = %+v, %v", status, err)
	}
	if status.Supported || status.UpdateAvailable {
		t.Fatalf("a development build was offered the update: %+v", status)
	}
	if _, err := service.Update(t.Context()); err != ErrUpdateUnsupported {
		t.Fatalf("update of a development build = %v", err)
	}
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
