// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != "log:separate" || first.Next == nil {
		t.Fatal(first, err)
	}
	second, err := db.ListPanelLogs(ctx, PanelLogFilter{Limit: 1, Cursor: first.Next, Since: &now})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "log:worker" || second.Next != nil {
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
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].ID != "log:text-match" || first.Next == nil {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	filter.Offset = 1
	second, err := db.ListPanelLogs(ctx, filter)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID != "log:code-match" {
		t.Fatalf("offset page: %+v, %v", second, err)
	}
	filter.Offset, filter.Cursor = 0, first.Next
	second, err = db.ListPanelLogs(ctx, filter)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID != "log:code-match" {
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

func TestPanelLogsExposeHistoricalPublicRuntimeContext(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	now := time.Now().UTC().Add(time.Minute)
	bundle, observation := seedAppliedRuntime(t, ctx, db, now)
	now = observation.StartedAt.Add(time.Minute)
	input := runtimeTransitionTestInput("panel-detail", RuntimeTransitionUnknown, "termination_result_uncertain", bundle.ID, observation, now.Add(time.Second))
	input.UncertainSince = &now
	if _, err := db.AppendRuntimeTransition(ctx, input); err != nil {
		t.Fatal(err)
	}
	page, err := db.ListPanelLogs(ctx, PanelLogFilter{SearchCodes: []string{input.Reason}})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("panel logs: %+v, %v", page, err)
	}
	item := page.Items[0]
	var metadata map[string]any
	if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if item.Source != "runtime" || item.Status != "unknown" || len(metadata) != 5 || metadata["activation_bundle_id"] != bundle.ID || metadata["pid"] != float64(observation.PID) || metadata["generation"] != float64(input.Generation) || metadata["process_started_at"] != formatTime(*input.ProcessStartedAt) || metadata["uncertain_since"] != formatTime(now) {
		t.Fatalf("historical context: %+v, %s", item, item.Metadata)
	}
	if strings.Contains(string(item.Metadata), "process_start_token") || strings.Contains(string(item.Metadata), input.ProcessStartToken) {
		t.Fatalf("internal evidence leaked: %s", item.Metadata)
	}
	page, err = db.ListPanelLogs(ctx, PanelLogFilter{SearchCodes: []string{"history_initialized"}})
	if err != nil || len(page.Items) != 1 || strings.Contains(string(page.Items[0].Metadata), `"pid"`) {
		t.Fatalf("missing historical evidence fabricated: %+v, %v", page, err)
	}
}
