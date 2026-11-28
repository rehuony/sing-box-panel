// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
)

func TestPanelLogsPreserveEventsAndCursor(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	now := time.Now().UTC()
	for _, entry := range []LogEntry{
		{ID: "worker", Time: now.Add(time.Second), Source: LogSourcePanel, Level: LogLevelInfo, Code: "catalog.completed", Message: "Catalog refreshed", Metadata: json.RawMessage(`{}`)},
		{ID: "separate", Time: now.Add(2 * time.Second), Source: LogSourceSecurity, Level: LogLevelWarn, Code: "auth.rejected", Message: "authentication rejected", Metadata: json.RawMessage(`{}`)},
	} {
		if _, err := db.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.ListPanelLogs(ctx, PanelLogFilter{Limit: 1, Since: &now})
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != "separate" || first.Next == nil {
		t.Fatal(first, err)
	}
	second, err := db.ListPanelLogs(ctx, PanelLogFilter{Limit: 1, Cursor: first.Next, Since: &now})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "worker" || second.Next != nil {
		t.Fatal(second, err)
	}
	filtered, err := db.ListPanelLogs(ctx, PanelLogFilter{Search: "authentication", Level: "warn"})
	if err != nil || len(filtered.Items) != 1 {
		t.Fatal(filtered, err)
	}
}

func TestPanelLogsSearchAlternativesShareTotalsAndPagination(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	now := time.Now().UTC().Add(time.Minute)
	for i, entry := range []LogEntry{
		{ID: "code-match", Level: LogLevelInfo, Code: "runtime.start.completed", Message: "Core start completed"},
		{ID: "text-match", Level: LogLevelInfo, Code: "custom.event", Message: "核心启动 custom diagnostic"},
		{ID: "wrong-level", Level: LogLevelError, Code: "runtime.start.completed", Message: "Failed"},
		{ID: "unrelated", Level: LogLevelInfo, Code: "panel.ready", Message: "Panel ready"},
	} {
		entry.Time, entry.Source = now.Add(time.Duration(i)*time.Second), LogSourcePanel
		if _, err := db.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	until := now.Add(4 * time.Second)
	filter := PanelLogFilter{Search: "核心启动", SearchCodes: []string{"runtime.start.completed"}, Level: "info", Since: &now, Until: &until, Limit: 1}
	first, err := db.ListPanelLogs(ctx, filter)
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].ID != "text-match" || first.Next == nil {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	filter.Offset = 1
	second, err := db.ListPanelLogs(ctx, filter)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID != "code-match" {
		t.Fatalf("offset page: %+v, %v", second, err)
	}
	filter.Offset, filter.Cursor = 0, first.Next
	second, err = db.ListPanelLogs(ctx, filter)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID != "code-match" {
		t.Fatalf("cursor page: %+v, %v", second, err)
	}
	filter.Cursor, filter.Search, filter.SearchCodes = nil, "CORE START", nil
	legacy, err := db.ListPanelLogs(ctx, filter)
	if err != nil || legacy.Total != 1 {
		t.Fatalf("legacy search: %+v, %v", legacy, err)
	}
	filter.Search, filter.SearchCodes = "", []string{"runtime.start.completed"}
	codeOnly, err := db.ListPanelLogs(ctx, filter)
	if err != nil || codeOnly.Total != 1 {
		t.Fatalf("code-only search: %+v, %v", codeOnly, err)
	}
}

// TestPanelLogScopesKeepEntriesFromBeforeTheNodeIndex: entries a log held
// before its node column arrived keep their node or panel scope.
func TestPanelLogScopesKeepEntriesFromBeforeTheNodeIndex(t *testing.T) {
	ctx := testContext(t)
	dir := t.TempDir()
	schema, err := LogSchema()
	if err != nil {
		t.Fatal(err)
	}
	baseline := database.Schema{ApplicationID: schema.ApplicationID, Migrations: schema.Migrations[:1]}
	logs, err := database.Open(ctx, filepath.Join(dir, LogsFile), baseline, false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, entry := range [][2]string{{"about-node", `{"node_id":"node_a"}`}, {"about-panel", `{"node_id":""}`}, {"unnamed", `{}`}} {
		if _, err := logs.ExecContext(ctx, `INSERT INTO log_entries(id, occurred_at, source, level, code, message, metadata_json)
            VALUES (?, ?, 'panel', 'info', 'panel.ready', 'Panel ready', ?)`, entry[0], formatTime(now), entry[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := logs.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, filepath.Join(dir, PanelFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if page, err := db.ListPanelLogs(ctx, PanelLogFilter{NodeID: "node_a"}); err != nil || page.Total != 1 || page.Items[0].ID != "about-node" {
		t.Fatalf("node scope: %+v, %v", page, err)
	}
	if page, err := db.ListPanelLogs(ctx, PanelLogFilter{PanelOnly: true}); err != nil || page.Total != 2 {
		t.Fatalf("panel scope: %+v, %v", page, err)
	}
}

func TestFleetRuntimeHistoryNamesNodesAndFilters(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	if _, err := enrollTestNode(ctx, db, "node_other", "edge-tokyo", nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	observation := runtimeProcess{PID: 4242, StartedAt: now}
	for index, input := range []RuntimeTransitionInput{
		runtimeTransitionTestInput(1, RuntimeTransitionRunning, "start_requested", "bundle-a", observation, now.Add(time.Second)),
		runtimeTransitionTestInput(2, RuntimeTransitionFailed, "process_exited", "bundle-a", observation, now.Add(2*time.Second)),
		{NodeID: "node_other", InstanceID: "instance-b", SourceID: 1, State: RuntimeTransitionStopped, Reason: "stop_requested", OccurredAt: now.Add(3 * time.Second)},
	} {
		if err := db.RecordRuntimeTransition(ctx, input); err != nil {
			t.Fatalf("transition %d: %v", index, err)
		}
	}
	page, err := db.ListFleetRuntimeTransitions(ctx, FleetRuntimeHistoryFilter{Limit: 2})
	if err != nil || page.Total != 3 || len(page.Items) != 2 || page.Items[0].NodeName != "edge-tokyo" || page.Items[1].Reason != "process_exited" {
		t.Fatalf("newest first: %+v, %v", page, err)
	}
	if page, err = db.ListFleetRuntimeTransitions(ctx, FleetRuntimeHistoryFilter{Limit: 2, Offset: 2}); err != nil || len(page.Items) != 1 || page.Items[0].PID != observation.PID {
		t.Fatalf("second page: %+v, %v", page, err)
	}
	if page, err = db.ListFleetRuntimeTransitions(ctx, FleetRuntimeHistoryFilter{Search: "TOKYO"}); err != nil || page.Total != 1 || page.Items[0].NodeID != "node_other" {
		t.Fatalf("node name search: %+v, %v", page, err)
	}
	if page, err = db.ListFleetRuntimeTransitions(ctx, FleetRuntimeHistoryFilter{NodeID: testNodeID, State: RuntimeTransitionFailed, SearchCodes: []string{"process_exited"}}); err != nil || page.Total != 1 {
		t.Fatalf("node, state and reason: %+v, %v", page, err)
	}
}
