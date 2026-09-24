// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/corelogs"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestLogApplicationRecordsListsTailsAndDeletes(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 2, 3, 4, 0, time.UTC)
	app.now = func() time.Time { return now }
	app.random = func(destination []byte) (int, error) {
		for index := range destination {
			destination[index] = byte(index + 1)
		}
		return len(destination), nil
	}

	recorded, err := app.RecordLog(ctx, LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "catalog.started",
		Message: "catalog refresh started", Metadata: json.RawMessage(`{"token":"plaintext","operation_id":"catalog_1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.ID != "log_0102030405060708090a0b0c0d0e0f10" || !recorded.Time.Equal(now) {
		t.Fatalf("recorded=%+v", recorded)
	}
	if string(recorded.Metadata) != `{"operation_id":"catalog_1","token":"[REDACTED]"}` {
		t.Fatalf("metadata=%s", recorded.Metadata)
	}

	page, err := app.ListLogs(ctx, LogListRequest{Source: store.LogSourcePanel, Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != recorded.ID {
		t.Fatalf("page=%+v error=%v", page, err)
	}
	tail, err := app.TailLogs(ctx, LogTailRequest{Since: &now, Limit: 10})
	if err != nil || len(tail) != 1 || tail[0].ID != recorded.ID {
		t.Fatalf("tail=%+v error=%v", tail, err)
	}

	deleted, err := app.DeleteLog(ctx, recorded.ID)
	if err != nil || !deleted.Deleted || deleted.ID != recorded.ID {
		t.Fatalf("deleted=%+v error=%v", deleted, err)
	}
	if _, err := app.Log(ctx, recorded.ID); !IsLogNotFound(err) || !errors.Is(err, store.ErrLogEntryNotFound) {
		t.Fatalf("missing error=%v", err)
	}
}

func TestOperationLogContextAndSafeFailureClassification(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	for _, test := range []struct {
		name string
		err  error
		code string
	}{
		{"success", nil, ""},
		{"known", fmt.Errorf("password=secret configuration payload: %w", store.ErrCoreNotEnabled), "core_not_enabled"},
		{"unknown", errors.New("https://user:secret@example.com/private-subscription-token"), "operation_failed"},
		{"canceled", context.Canceled, "canceled"},
		{"file", corelogs.ErrCurrentFile, "core_log_current"},
		{"check", fmt.Errorf("configuration payload password=secret: %w", coreruntime.ErrCheckFailed), "core_check_failed"},
		{"health", errors.Join(coreruntime.ErrRuntime, coreruntime.ErrHealthFailed, errors.New("private-subscription-token")), "core_health_failed"},
		{"version", fmt.Errorf("secret binary output: %w", coreruntime.ErrVersionMismatch), "core_version_mismatch"},
		{"termination", errors.Join(coreruntime.ErrRuntime, coreruntime.ErrTermination), "core_termination_failed"},
		{"check_canceled", errors.Join(coreruntime.ErrCheckFailed, context.Canceled), "canceled"},
		{"health_deadline", errors.Join(coreruntime.ErrHealthFailed, context.DeadlineExceeded), "deadline"},
	} {
		t.Run(test.name, func(t *testing.T) {
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			app.RecordOperation(canceled, "test."+test.name, "Test operation", test.err, OperationLogContext{
				StartedAt: time.Now(), CoreID: "core-test", Generation: 3,
				StartupArtifactID: "startup-test", ActivationBundleID: "bundle-test",
			})
			page, err := app.PanelLogs(ctx, store.PanelLogFilter{Search: "test." + test.name})
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("operation log lost: %+v, %v", page, err)
			}
			var details map[string]any
			if err := json.Unmarshal(page.Items[0].Metadata, &details); err != nil {
				t.Fatal(err)
			}
			if details["core_id"] != "core-test" || details["generation"] != float64(3) || details["startup_artifact_id"] != "startup-test" || details["activation_bundle_id"] != "bundle-test" {
				t.Fatalf("context: %v", details)
			}
			if duration, ok := details["duration_ms"].(float64); !ok || duration < 0 {
				t.Fatalf("duration missing: %v", details)
			}
			if test.code == "" && details["error_code"] != nil || test.code != "" && details["error_code"] != test.code {
				t.Fatalf("classification: %v", details)
			}
			if raw := string(page.Items[0].Metadata); strings.Contains(raw, "secret") || strings.Contains(raw, "payload") || strings.Contains(raw, "private-subscription") {
				t.Fatalf("unsafe context: %s", raw)
			}
		})
	}
}

func TestCoreLogActionsRecordOnlyManagedTargetAndDuration(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	app.settings.DataDir = t.TempDir()
	files, err := app.coreLogFiles()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Writer().Write([]byte("INFO sample\n")); err != nil {
		t.Fatal(err)
	}
	listed, err := files.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("files: %v, %v", listed, err)
	}
	if err := app.ClearCoreLog(ctx, listed[0].Name); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteCoreLogFile(ctx, listed[0].Name); !errors.Is(err, corelogs.ErrCurrentFile) {
		t.Fatal(err)
	}
	if err := app.ClearCoreLog(ctx, "../../secret"); !errors.Is(err, corelogs.ErrInvalidFile) {
		t.Fatal(err)
	}
	page, err := app.PanelLogs(ctx, store.PanelLogFilter{Search: "core.log."})
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("logs: %+v, %v", page, err)
	}
	for _, item := range page.Items {
		var details map[string]any
		if err := json.Unmarshal(item.Metadata, &details); err != nil {
			t.Fatal(err)
		}
		if details["duration_ms"] == nil {
			t.Fatalf("missing duration: %v", details)
		}
		if details["error_code"] == "core_log_invalid" {
			if details["file"] != nil {
				t.Fatalf("invalid target persisted: %v", details)
			}
		} else if details["file"] != listed[0].Name {
			t.Fatalf("target not recorded: %v", details)
		}
	}
}

func TestExplicitLogClearUsesStrictCutoff(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }

	for _, entry := range []store.LogEntry{
		{ID: "log_expired", Time: now.Add(-8 * 24 * time.Hour), Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "test.expired", Message: "expired", Metadata: json.RawMessage(`{}`)},
		{ID: "log_at_cutoff", Time: now.Add(-7 * 24 * time.Hour), Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "test.cutoff", Message: "kept", Metadata: json.RawMessage(`{}`)},
		{ID: "log_recent", Time: now.Add(-time.Hour), Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "test.recent", Message: "kept", Metadata: json.RawMessage(`{}`)},
	} {
		if _, err := database.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}

	cutoff := now.Add(-7 * 24 * time.Hour)
	result, err := app.ClearLogs(ctx, LogClearRequest{Before: &cutoff})
	if err != nil || result.Deleted != 1 {
		t.Fatalf("ClearLogs() = %+v, %v", result, err)
	}
	page, err := database.ListLogEntries(ctx, store.LogListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != "log_recent" || page.Items[1].ID != "log_at_cutoff" {
		t.Fatalf("retained logs = %+v", page.Items)
	}
}

func TestPanelEventsSurviveSettingsChangesAndBackupRestore(t *testing.T) {
	app := panelFileApp(t)
	ctx := t.Context()
	entry, err := app.database.AppendLogEntry(ctx, store.LogEntry{
		ID: "historical_panel_event", Time: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "test.history",
		Message: "Historical panel event", Metadata: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := app.PanelSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	view.Service.CoreLogRetentionDays = new(1)
	saved, err := app.SavePanelSettings(ctx, PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Service: &view.Service})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := app.ExportPanelBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	file, err := app.ConfigurationFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RestorePanelBackup(ctx, PanelRestoreRequest{
		Backup: backup, SettingsRevision: saved.Revision, ConfigurationRevision: file.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	page, err := app.PanelLogs(ctx, store.PanelLogFilter{Search: "Historical panel event"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "log:"+entry.ID {
		t.Fatalf("historical panel event was lost: %+v, %v", page, err)
	}
}
