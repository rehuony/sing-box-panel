// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestHistoryDiscoveryNeverAuthorizesRemoval(t *testing.T) {
	config, data := fixture(t)
	history := filepath.Join(t.TempDir(), "state", "cleanup-history.json")
	record := CleanupHistory{SettingsPath: config, DataDirs: []string{data, data}, Scope: "user", Outcome: "started"}
	if err := WriteCleanupHistory(t.Context(), history, record); err != nil {
		t.Fatal(err)
	}
	report, err := InspectWithHistory(t.Context(), config, history)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(history)
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(data, "new-owner"))
	report, err = InspectWithHistory(t.Context(), config, history)
	if err != nil || report.DataDir != "" || report.History == nil || len(report.History.DataDirs) != 1 {
		t.Fatalf("history inventory: %+v %v", report, err)
	}
	var historical bool
	for _, e := range report.Entries {
		if e.Path == data {
			historical = e.Cleanup == "retain"
		}
	}
	if !historical {
		t.Fatal("historical directory is not retained")
	}
	if _, err := Clean(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "new-owner")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(history)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only inspection or no-op rewrote history")
	}
	for path, mode := range map[string]os.FileMode{history: 0600, filepath.Dir(history): 0700} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("permissions %s: %v", path, err)
		}
	}
}

func TestCleanupMissingConfigurationUsesLocation(t *testing.T) {
	config, data := fixture(t)
	if err := settings.RememberDataLocation(config, data); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(t.Context(), config)
	if err != nil || report.DataDir != data {
		t.Fatalf("location recovery: %+v %v", report, err)
	}
	if _, err := Clean(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{data, config + ".location", config + ".lock"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("leftover %s: %v", path, err)
		}
	}
}

func TestMissingDataStillFinalizesDedicatedConfigDirectory(t *testing.T) {
	for _, extra := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "extra"}[extra], func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "sing-box-panel", "setting.json")
			data := filepath.Join(root, "missing-data")
			if _, err := settings.EnsureFile(t.Context(), config, data); err != nil {
				t.Fatal(err)
			}
			extraPath := filepath.Join(filepath.Dir(config), "keep.txt")
			if extra {
				putFile(t, extraPath)
			}
			report, err := Inspect(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Clean(t.Context(), report)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(result.Removed, config) || !slices.Contains(result.Removed, config+".location") {
				t.Fatalf("missing confirmed results: %+v", result)
			}
			_, err = os.Stat(filepath.Dir(config))
			if extra {
				if err != nil || !slices.Contains(result.Retained, filepath.Dir(config)) {
					t.Fatalf("extra content lost: %+v %v", result, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("empty config directory retained")
			}
		})
	}
}

func TestHistoryRejectsLinksInvalidRecordsAndContainedState(t *testing.T) {
	config, data := fixture(t)
	report, err := Inspect(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	report.HistoryPath = filepath.Join(data, "state", "cleanup-history.json")
	if err := ValidateCleanup(report); err == nil {
		t.Fatal("history inside data accepted")
	}
	history := filepath.Join(t.TempDir(), "cleanup-history.json")
	if err := os.Symlink(config, history); err != nil {
		t.Fatal(err)
	}
	record := CleanupHistory{SettingsPath: config, DataDirs: []string{data}, Outcome: "started"}
	if err := WriteCleanupHistory(t.Context(), history, record); err == nil {
		t.Fatal("history symlink accepted")
	}
	if err := os.Remove(history); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history, []byte(`{"version":1,"instances":{"/invalid":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectWithHistory(t.Context(), config, history); err == nil {
		t.Fatal("invalid history accepted")
	}
}

func TestHistoryKeepsMultipleInstancesAndRefusesConcurrentWriter(t *testing.T) {
	config, data := fixture(t)
	other, otherData := fixture(t)
	history := filepath.Join(t.TempDir(), "state", "cleanup-history.json")
	for _, record := range []CleanupHistory{{SettingsPath: config, DataDirs: []string{data}, Outcome: "started"}, {SettingsPath: other, DataDirs: []string{otherData}, Outcome: "completed"}} {
		if err := WriteCleanupHistory(t.Context(), history, record); err != nil {
			t.Fatal(err)
		}
	}
	document, err := readHistory(history)
	if err != nil || len(document.Instances) != 2 {
		t.Fatalf("lost instance record: %+v %v", document, err)
	}
	before, _ := os.ReadFile(history)
	lock, err := store.LockDirectoryForCleanup(filepath.Dir(history))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := WriteCleanupHistory(t.Context(), history, CleanupHistory{SettingsPath: config, Outcome: "completed"}); err == nil {
		t.Fatal("concurrent history writer acquired lock")
	}
	after, _ := os.ReadFile(history)
	if !bytes.Equal(before, after) {
		t.Fatal("busy history was modified")
	}
}
