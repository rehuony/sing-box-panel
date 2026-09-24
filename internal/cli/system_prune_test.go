// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestPruneDFRepeatAndReappearingHistory(t *testing.T) {
	config := commandSettingsFixture(t)
	data, err := settings.LoadDataDir(config)
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeSystemdService{err: panelSystemd.ErrUnsupportedOS}
	run := func(args ...string) string {
		t.Helper()
		args = append(args, "-c", config, "-o", "json")
		out, _, err := executeSystemCommand(t, service, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	run("system", "prune", "--yes")
	history := testHistoryPath(t)
	before, _ := os.ReadFile(history)
	info, _ := os.Stat(history)
	for _, args := range [][]string{{"system", "df"}, {"system", "prune"}, {"system", "prune", "--yes"}, {"system", "df"}} {
		run(args...)
	}
	after, _ := os.ReadFile(history)
	latest, _ := os.Stat(history)
	if !bytes.Equal(before, after) || !info.ModTime().Equal(latest.ModTime()) {
		t.Fatal("read-only/no-op changed history")
	}
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(data, "new-owner")
	if err := os.WriteFile(foreign, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	var report instanceFilesReport
	if err := json.Unmarshal([]byte(run("system", "df")), &report); err != nil {
		t.Fatal(err)
	}
	if report.History == nil || report.History.Outcome != "completed" || report.DataDir != "" {
		t.Fatalf("post-prune report: %+v", report)
	}
	var found bool
	for _, entry := range report.Entries {
		if entry.Path == data {
			found = entry.Cleanup == "retain" && strings.Contains(entry.Role, "historical")
		}
	}
	if !found {
		t.Fatal("historical path not visible")
	}
	var result installation.CleanupResult
	if err := json.Unmarshal([]byte(run("system", "prune", "--yes")), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 || !slices.Contains(result.Retained, data) {
		t.Fatalf("historical deletion authorized: %+v", result)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal(err)
	}
}

func TestPruneHistoryWriteFailurePrecedesServiceMutation(t *testing.T) {
	config := commandSettingsFixture(t)
	history := testHistoryPath(t)
	if err := os.MkdirAll(filepath.Dir(history), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(config, history); err != nil {
		t.Fatal(err)
	}
	service := &fakeSystemdService{}
	_, _, err := executeSystemCommand(t, service, "system", "prune", "--yes", "-c", config)
	if err == nil {
		t.Fatal("unsafe history accepted")
	}
	if service.uninstallRequest.Scope != "" {
		t.Fatal("service mutated before validating history")
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatal(err)
	}
}

type historyFailureService struct {
	fakeSystemdService
	afterUninstall func()
}

func (service *historyFailureService) Uninstall(ctx context.Context, request panelSystemd.UninstallRequest) (panelSystemd.UninstallResult, error) {
	service.afterUninstall()
	return service.fakeSystemdService.Uninstall(ctx, request)
}

func TestFinalHistoryFailurePreservesCompletedResult(t *testing.T) {
	config := commandSettingsFixture(t)
	history := testHistoryPath(t)
	service := &historyFailureService{fakeSystemdService: fakeSystemdService{
		filesResult:  panelSystemd.FilesResult{Scope: panelSystemd.ScopeUser, SettingsPath: config},
		statusResult: panelSystemd.Status{UnitFileSettingsPath: config},
	}, afterUninstall: func() {
		// A concurrent replacement makes the final history write fail without
		// adding deletion authority or losing the already completed file removals.
		if err := os.Remove(history); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(t.TempDir(), "foreign"), history); err != nil {
			t.Fatal(err)
		}
	}}
	out, _, err := executeSystemCommand(t, service, "system", "prune", "--yes", "-c", config, "-o", "json")
	if err == nil {
		t.Fatal("final history failure reported success")
	}
	var result installation.CleanupResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Removed, config) || len(result.Warnings) == 0 {
		t.Fatalf("lost completed cleanup: %+v", result)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("instance cleanup did not finish")
	}
}
