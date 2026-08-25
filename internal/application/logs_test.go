// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
		Source: store.LogSourceTask, Level: store.LogLevelInfo, Code: "task.started",
		Message: "task started", Metadata: json.RawMessage(`{"token":"plaintext","task_id":"task_1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.ID != "log_0102030405060708090a0b0c0d0e0f10" || !recorded.Time.Equal(now) {
		t.Fatalf("recorded=%+v", recorded)
	}
	if string(recorded.Metadata) != `{"task_id":"task_1","token":"[REDACTED]"}` {
		t.Fatalf("metadata=%s", recorded.Metadata)
	}

	page, err := app.ListLogs(ctx, LogListRequest{Source: store.LogSourceTask, Limit: 10})
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

func TestEnforceLogRetentionUsesConfiguredDaysAndStrictCutoff(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	app.settings.Logs.RetentionDays = 7
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

	result, err := app.EnforceLogRetention(ctx)
	if err != nil || result.Deleted != 1 {
		t.Fatalf("EnforceLogRetention() = %+v, %v", result, err)
	}
	page, err := database.ListLogEntries(ctx, store.LogListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != "log_recent" || page.Items[1].ID != "log_at_cutoff" {
		t.Fatalf("retained logs = %+v", page.Items)
	}
}
