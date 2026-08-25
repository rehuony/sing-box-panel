// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestDurableLogCommandsListShowTailDeleteAndClear(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	base := time.Date(2026, time.August, 26, 8, 0, 0, 0, time.UTC)
	appendCLILog(t, database, "log_a", base, store.LogSourcePanel)
	appendCLILog(t, database, "log_b", base.Add(time.Second), store.LogSourceCore)
	open := sharedLogApplication(database)

	listOutput := executeDurableLogCommand(t, ctx, open, outputJSON,
		"list", "--source=core", "--limit=10",
	)
	var page application.LogPage
	if err := json.Unmarshal(listOutput, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "log_b" {
		t.Fatalf("page=%+v", page)
	}

	showOutput := executeDurableLogCommand(t, ctx, open, outputJSON, "show", "log_a")
	var shown store.LogEntry
	if err := json.Unmarshal(showOutput, &shown); err != nil || shown.ID != "log_a" {
		t.Fatalf("shown=%+v error=%v", shown, err)
	}

	tailOutput := executeDurableLogCommand(t, ctx, open, outputJSONL,
		"tail", "--since="+base.Format(time.RFC3339Nano), "--limit=10",
	)
	lines := strings.Split(strings.TrimSpace(string(tailOutput)), "\n")
	if len(lines) != 2 {
		t.Fatalf("tail output=%q", tailOutput)
	}
	for index, want := range []string{"log_a", "log_b"} {
		var entry store.LogEntry
		if err := json.Unmarshal([]byte(lines[index]), &entry); err != nil || entry.ID != want {
			t.Fatalf("line %d entry=%+v error=%v", index, entry, err)
		}
	}
	resumedOutput := executeDurableLogCommand(t, ctx, open, outputJSONL,
		"tail", "--cursor-time="+base.Format(time.RFC3339Nano), "--cursor-id=log_a", "--limit=10",
	)
	var resumed store.LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(resumedOutput), &resumed); err != nil || resumed.ID != "log_b" {
		t.Fatalf("resumed=%+v error=%v output=%s", resumed, err, resumedOutput)
	}

	deleteOutput := executeDurableLogCommand(t, ctx, open, outputJSON, "delete", "log_a")
	var deleted application.LogDeleteResult
	if err := json.Unmarshal(deleteOutput, &deleted); err != nil || !deleted.Deleted || deleted.ID != "log_a" {
		t.Fatalf("deleted=%+v error=%v", deleted, err)
	}

	clearOutput := executeDurableLogCommand(t, ctx, open, outputJSON, "clear", "--source=core", "--all")
	var cleared application.LogClearResult
	if err := json.Unmarshal(clearOutput, &cleared); err != nil || cleared.Deleted != 1 {
		t.Fatalf("cleared=%+v error=%v", cleared, err)
	}
}

func TestDurableLogClearRequiresExplicitUnboundedAcknowledgement(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	command := newDurableLogCommand(&options{format: outputJSON}, sharedLogApplication(database))
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{"clear"})
	err = command.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--all") {
		t.Fatalf("error=%v exit=%d", err, ExitCode(err))
	}
}

func TestDurableLogTailFollowStreamsJSONLAndCancelsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	appendCLILog(t, database, "log_initial", time.Now().UTC(), store.LogSourceTask)

	writer := &signalingWriter{ready: make(chan struct{})}
	command := newDurableLogCommand(&options{format: outputJSONL}, sharedLogApplication(database))
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetOut(writer)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"tail", "--follow", "--poll-interval=50ms", "--limit=10"})
	result := make(chan error, 1)
	go func() { result <- command.ExecuteContext(ctx) }()
	select {
	case <-writer.ready:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("tail did not emit its initial JSONL entry")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("tail cancellation error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail did not stop after cancellation")
	}
	var entry store.LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(writer.Bytes()), &entry); err != nil || entry.ID != "log_initial" {
		t.Fatalf("entry=%+v error=%v output=%s", entry, err, writer.Bytes())
	}
}

func appendCLILog(t *testing.T, database *store.Store, id string, at time.Time, source store.LogSource) {
	t.Helper()
	if _, err := database.AppendLogEntry(context.Background(), store.LogEntry{
		ID: id, Time: at, Source: source, Level: store.LogLevelInfo,
		Code: "test.event", Message: "test event", Metadata: json.RawMessage(`{"safe":true}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func sharedLogApplication(database *store.Store) openApplicationFunc {
	return func(context.Context, string) (*application.Application, error) {
		return application.FromStore(database), nil
	}
}

func executeDurableLogCommand(
	t *testing.T,
	ctx context.Context,
	open openApplicationFunc,
	format outputFormat,
	args ...string,
) []byte {
	t.Helper()
	var output bytes.Buffer
	command := newDurableLogCommand(&options{format: format}, open)
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	if err := command.ExecuteContext(ctx); err != nil {
		t.Fatalf("log %v error=%v output=%s", args, err, output.String())
	}
	return output.Bytes()
}

type signalingWriter struct {
	mu    sync.Mutex
	data  bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (writer *signalingWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	written, err := writer.data.Write(data)
	writer.once.Do(func() { close(writer.ready) })
	return written, err
}

func (writer *signalingWriter) Bytes() []byte {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return bytes.Clone(writer.data.Bytes())
}
