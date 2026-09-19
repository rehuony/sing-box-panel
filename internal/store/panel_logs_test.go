// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPanelLogsDeduplicateTasksAndPreserveCursor(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	now := time.Now().UTC()
	task, err := db.EnqueueTask(ctx, EnqueueTaskInput{ID: "task-panel", Lane: TaskLaneMaintenance, Kind: TaskKindCatalogRefresh, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []LogEntry{
		{ID: "worker", Time: now.Add(time.Second), Source: LogSourceTask, Level: LogLevelInfo, Code: "task.queued", Message: "worker queued", Metadata: json.RawMessage(`{"task_id":"task-panel"}`)},
		{ID: "separate", Time: now.Add(2 * time.Second), Source: LogSourceSecurity, Level: LogLevelWarn, Code: "auth.rejected", Message: "authentication rejected", Metadata: json.RawMessage(`{}`)},
	} {
		if _, err = db.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.ListPanelLogs(ctx, PanelLogFilter{Limit: 1, Since: &now})
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != "log:separate" || first.Next == nil {
		t.Fatal(first, err)
	}
	second, err := db.ListPanelLogs(ctx, PanelLogFilter{Limit: 1, Cursor: first.Next, Since: &now})
	if err != nil || len(second.Items) != 1 || second.Items[0].TaskID != task.ID || second.Next != nil {
		t.Fatal(second, err)
	}
	filtered, err := db.ListPanelLogs(ctx, PanelLogFilter{Search: "authentication", Level: "warn"})
	if err != nil || len(filtered.Items) != 1 {
		t.Fatal(filtered, err)
	}
}
