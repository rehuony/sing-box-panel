// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAppendLogEntrySanitizesSecretsAndBodiesBeforePersistence(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 1, 2, 3, 4, time.UTC)
	entry, err := database.AppendLogEntry(ctx, LogEntry{
		ID:      "log_safe",
		Time:    now,
		Source:  LogSourceSecurity,
		Level:   LogLevelWarn,
		Code:    "auth.rejected",
		Message: "Authorization: Bearer very-secret-value",
		Metadata: json.RawMessage(`{
            "request_id":"req-1",
			"connection":{"access_token":"abc","proxy_username":"alice","safe":true},
			"private-key":"key-material",
			"proxy_url":"socks5://bob:proxy-pass@example.test:1080",
			"headers":"Cookie: session=cookie-one; preference=cookie-two\nX-Request-ID: req-1",
            "config_json":{"outbounds":[{"password":"inside-body"}]},
            "subscription_body":"secret subscription"
        }`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(entry.Message, "very-secret-value") || !strings.Contains(entry.Message, redactedLogValue) {
		t.Fatalf("message was not sanitized: %q", entry.Message)
	}
	var metadata map[string]any
	if err := json.Unmarshal(entry.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	credentials := metadata["connection"].(map[string]any)
	if credentials["access_token"] != redactedLogValue || credentials["proxy_username"] != redactedLogValue || credentials["safe"] != true {
		t.Fatalf("credentials=%#v", credentials)
	}
	if metadata["private-key"] != redactedLogValue || metadata["config_json"] != omittedLogValue || metadata["subscription_body"] != omittedLogValue {
		t.Fatalf("metadata=%#v", metadata)
	}

	var persistedMessage, persistedMetadata string
	if err := database.db.QueryRowContext(
		ctx,
		`SELECT message, metadata_json FROM log_entries WHERE id = ?`,
		entry.ID,
	).Scan(&persistedMessage, &persistedMetadata); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"very-secret-value", "abc", "alice", "key-material", "bob:proxy-pass", "cookie-one", "cookie-two", "inside-body", "secret subscription"} {
		if strings.Contains(persistedMessage, forbidden) || strings.Contains(persistedMetadata, forbidden) {
			t.Fatalf("persisted log contains %q: message=%q metadata=%s", forbidden, persistedMessage, persistedMetadata)
		}
	}
}

func TestAppendLogEntryValidatesStrictBoundsAndJSON(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	valid := LogEntry{
		ID: "log_valid", Time: time.Now(), Source: LogSourcePanel, Level: LogLevelInfo,
		Code: "panel.ready", Message: "ready", Metadata: json.RawMessage(`{}`),
	}
	tests := []struct {
		name   string
		mutate func(*LogEntry)
	}{
		{name: "invalid id", mutate: func(entry *LogEntry) { entry.ID = "not valid" }},
		{name: "zero time", mutate: func(entry *LogEntry) { entry.Time = time.Time{} }},
		{name: "invalid source", mutate: func(entry *LogEntry) { entry.Source = "journal" }},
		{name: "invalid level", mutate: func(entry *LogEntry) { entry.Level = "notice" }},
		{name: "invalid code", mutate: func(entry *LogEntry) { entry.Code = "Panel Ready" }},
		{name: "oversized message", mutate: func(entry *LogEntry) { entry.Message = strings.Repeat("m", MaximumLogMessageBytes+1) }},
		{name: "message control", mutate: func(entry *LogEntry) { entry.Message = "line one\nline two" }},
		{name: "subscription message", mutate: func(entry *LogEntry) { entry.Message = "vless://uuid@example.test:443" }},
		{name: "duplicate metadata key", mutate: func(entry *LogEntry) { entry.Metadata = json.RawMessage(`{"a":1,"a":2}`) }},
		{name: "metadata array", mutate: func(entry *LogEntry) { entry.Metadata = json.RawMessage(`[]`) }},
		{name: "full config metadata", mutate: func(entry *LogEntry) { entry.Metadata = json.RawMessage(`{"outbounds":[]}`) }},
		{name: "oversized metadata", mutate: func(entry *LogEntry) {
			entry.Metadata = json.RawMessage(`{"value":"` + strings.Repeat("x", MaximumLogMetadataBytes) + `"}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := valid
			entry.Metadata = append(json.RawMessage(nil), valid.Metadata...)
			test.mutate(&entry)
			if _, err := database.AppendLogEntry(ctx, entry); err == nil {
				t.Fatal("AppendLogEntry unexpectedly succeeded")
			}
		})
	}
}

func TestLogListTailAndKeysetPaginationAreDeterministic(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	base := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	entries := []LogEntry{
		logFixture("log_a", base, LogSourcePanel, LogLevelInfo),
		logFixture("log_c", base.Add(time.Second), LogSourceCore, LogLevelWarn),
		logFixture("log_b", base.Add(time.Second), LogSourceCore, LogLevelError),
		logFixture("log_d", base.Add(2*time.Second), LogSourceTask, LogLevelInfo),
	}
	for _, entry := range entries {
		if _, err := database.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}

	first, err := database.ListLogEntries(ctx, LogListFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := logIDs(first.Items); strings.Join(got, ",") != "log_d,log_c" || first.Next == nil {
		t.Fatalf("first page=%v next=%+v", got, first.Next)
	}
	second, err := database.ListLogEntries(ctx, LogListFilter{Cursor: first.Next, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := logIDs(second.Items); strings.Join(got, ",") != "log_b,log_a" || second.Next != nil {
		t.Fatalf("second page=%v next=%+v", got, second.Next)
	}

	core, err := database.ListLogEntries(ctx, LogListFilter{Source: LogSourceCore, Since: &base, Until: timePointer(base.Add(2 * time.Second)), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(logIDs(core.Items), ","); got != "log_c,log_b" {
		t.Fatalf("filtered=%s", got)
	}

	tailed, err := database.TailLogEntries(ctx, LogTailFilter{
		After: &LogCursor{Time: base.Add(time.Second), ID: "log_b"}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(logIDs(tailed), ","); got != "log_c,log_d" {
		t.Fatalf("tailed=%s", got)
	}
}

func TestLogDeleteAndClearReportExactEffects(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	base := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	for _, entry := range []LogEntry{
		logFixture("log_old_panel", base, LogSourcePanel, LogLevelInfo),
		logFixture("log_old_core", base, LogSourceCore, LogLevelInfo),
		logFixture("log_new_core", base.Add(time.Hour), LogSourceCore, LogLevelInfo),
	} {
		if _, err := database.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.DeleteLogEntry(ctx, "log_old_panel"); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteLogEntry(ctx, "log_old_panel"); !errors.Is(err, ErrLogEntryNotFound) {
		t.Fatalf("second delete error=%v", err)
	}
	cutoff := base.Add(30 * time.Minute)
	count, err := database.ClearLogEntries(ctx, LogClearFilter{Source: LogSourceCore, Before: &cutoff})
	if err != nil || count != 1 {
		t.Fatalf("filtered clear count=%d error=%v", count, err)
	}
	page, err := database.ListLogEntries(ctx, LogListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(logIDs(page.Items), ","); got != "log_new_core" {
		t.Fatalf("remaining=%s", got)
	}
	count, err = database.ClearLogEntries(ctx, LogClearFilter{})
	if err != nil || count != 1 {
		t.Fatalf("full clear count=%d error=%v", count, err)
	}
}

func logFixture(id string, at time.Time, source LogSource, level LogLevel) LogEntry {
	return LogEntry{
		ID: id, Time: at, Source: source, Level: level,
		Code: "test.event", Message: "test event", Metadata: json.RawMessage(`{"safe":true}`),
	}
}

func logIDs(entries []LogEntry) []string {
	result := make([]string, len(entries))
	for index, entry := range entries {
		result[index] = entry.ID
	}
	return result
}

func timePointer(value time.Time) *time.Time { return &value }
