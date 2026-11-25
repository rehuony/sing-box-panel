// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/logredact"
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
	if strings.Contains(entry.Message, "very-secret-value") || !strings.Contains(entry.Message, logredact.Redacted) {
		t.Fatalf("message was not sanitized: %q", entry.Message)
	}
	var metadata map[string]any
	if err := json.Unmarshal(entry.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	credentials := metadata["connection"].(map[string]any)
	if credentials["access_token"] != logredact.Redacted || credentials["proxy_username"] != logredact.Redacted || credentials["safe"] != true {
		t.Fatalf("credentials=%#v", credentials)
	}
	if metadata["private-key"] != logredact.Redacted || metadata["config_json"] != logredact.Omitted || metadata["subscription_body"] != logredact.Omitted {
		t.Fatalf("metadata=%#v", metadata)
	}

	var persistedMessage, persistedMetadata string
	if err := database.logs.QueryRowContext(
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
		{name: "oversized message", mutate: func(entry *LogEntry) { entry.Message = strings.Repeat("m", logredact.MaximumMessageBytes+1) }},
		{name: "message control", mutate: func(entry *LogEntry) { entry.Message = "line one\nline two" }},
		{name: "subscription message", mutate: func(entry *LogEntry) { entry.Message = "vless://uuid@example.test:443" }},
		{name: "duplicate metadata key", mutate: func(entry *LogEntry) { entry.Metadata = json.RawMessage(`{"a":1,"a":2}`) }},
		{name: "metadata array", mutate: func(entry *LogEntry) { entry.Metadata = json.RawMessage(`[]`) }},
		{name: "full config metadata", mutate: func(entry *LogEntry) { entry.Metadata = json.RawMessage(`{"outbounds":[]}`) }},
		{name: "oversized metadata", mutate: func(entry *LogEntry) {
			entry.Metadata = json.RawMessage(`{"value":"` + strings.Repeat("x", logredact.MaximumMetadataBytes) + `"}`)
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
		logFixture("log_d", base.Add(2*time.Second), LogSourcePanel, LogLevelInfo),
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

// A panel database of the previous schema holds the log; opening it moves
// every entry to the log database, also when an earlier start copied them
// and stopped before dropping the old table.
func TestOpenMovesLogEntriesOfThePreviousSchema(t *testing.T) {
	ctx := testContext(t)
	schema, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.Schema{ApplicationID: ApplicationID, Migrations: schema.Migrations[:1]}
	entries := []LogEntry{logFixture("log_first", time.Now().Add(-time.Hour), LogSourcePanel, LogLevelInfo), logFixture("log_second", time.Now(), LogSourceCore, LogLevelWarn)}
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprintf("interrupted=%t", interrupted), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "panel.db")
			old, err := database.Open(ctx, path, previous, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if _, err := old.ExecContext(ctx, `INSERT INTO log_entries(id, occurred_at, source, level, code, message, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					entry.ID, formatTime(entry.Time), entry.Source, entry.Level, entry.Code, entry.Message, string(entry.Metadata)); err != nil {
					t.Fatal(err)
				}
			}
			if err := old.Close(); err != nil {
				t.Fatal(err)
			}
			if interrupted {
				// The upgrade set the table aside and the copy committed; the drop did not run.
				panel, err := database.Open(ctx, path, schema, false)
				if err != nil {
					t.Fatal(err)
				}
				logSchema, err := LogSchema()
				if err != nil {
					t.Fatal(err)
				}
				logs, err := database.OpenBeside(ctx, panel, LogsFile, logSchema)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if _, err := logs.ExecContext(ctx, `INSERT INTO log_entries(id, occurred_at, source, level, code, message, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
						entry.ID, formatTime(entry.Time), entry.Source, entry.Level, entry.Code, entry.Message, string(entry.Metadata)); err != nil {
						t.Fatal(err)
					}
				}
				if err := errors.Join(logs.Close(), panel.Close()); err != nil {
					t.Fatal(err)
				}
			}
			upgraded, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			page, err := upgraded.ListLogEntries(ctx, LogListFilter{})
			if err != nil || strings.Join(logIDs(page.Items), ",") != "log_second,log_first" {
				t.Fatalf("moved entries = %v, %v", logIDs(page.Items), err)
			}
			if page.Items[0].Code != entries[1].Code || string(page.Items[0].Metadata) != string(entries[1].Metadata) {
				t.Fatalf("moved entry changed: %+v", page.Items[0])
			}
			var left int
			if err := upgraded.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name LIKE 'log_entries%'`).Scan(&left); err != nil || left != 0 {
				t.Fatalf("log objects left in the panel database: %d, %v", left, err)
			}
		})
	}
}

func TestClearingTheLogShrinksItsFiles(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	message := strings.Repeat("A long diagnostic message. ", 120)
	base := time.Now().Add(-time.Hour)
	for index := range 2000 {
		entry := logFixture(fmt.Sprintf("log_%04d", index), base.Add(time.Duration(index)*time.Millisecond), LogSourcePanel, LogLevelInfo)
		entry.Message = message
		if _, err := db.AppendLogEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	before, err := db.LogUsage(ctx)
	if err != nil || before.Entries != 2000 {
		t.Fatalf("usage before = %+v, %v", before, err)
	}
	if deleted, err := db.ClearLogEntries(ctx, LogClearFilter{}); err != nil || deleted != 2000 {
		t.Fatalf("cleared %d, %v", deleted, err)
	}
	if err := db.CompactLogs(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := db.LogUsage(ctx)
	if err != nil || after.Entries != 0 || after.Bytes >= before.Bytes/10 {
		t.Fatalf("usage after = %+v (before %+v), %v", after, before, err)
	}
}
