// SPDX-License-Identifier: GPL-3.0-or-later
package corelogs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClearPersistsAndPreservesAppendWriters(t *testing.T) {
	dataDir := t.TempDir()
	logs, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	output := logs.Writer()
	if _, err := output.Write([]byte("INFO old output\nERROR hidden by filters\n")); err != nil {
		t.Fatal(err)
	}
	name := "2026-09-23-000.log"
	for _, other := range []string{"2026-09-22-000.log", "2026-09-23-001.log"} {
		if err := os.WriteFile(filepath.Join(logs.dir, other), []byte("INFO unrelated\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A writer may already have opened the capture when HTTP clears it.
	opened, err := os.OpenFile(filepath.Join(logs.dir, name), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	before, err := opened.Stat()
	if err != nil {
		t.Fatal(err)
	}
	clearer, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := clearer.Clear(name); err != nil {
		t.Fatal(err)
	}
	reader, err := New(dataDir) // A fresh reader represents a page refresh or restart.
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int64{-1, 0} {
		chunk, err := reader.Read(name, offset, "")
		if err != nil || chunk.Text != "" || chunk.Size != 0 || chunk.NextOffset != 0 {
			t.Fatalf("persisted clear at %d: %+v, %v", offset, chunk, err)
		}
	}
	after, err := os.Stat(filepath.Join(logs.dir, name))
	if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0600 {
		t.Fatalf("clear replaced the file or its permissions: %v", err)
	}
	if _, err := opened.WriteString("INFO from open writer\n"); err != nil {
		t.Fatal(err)
	}
	chunk, err := reader.Read(name, 0, "")
	if err != nil || chunk.Text != "INFO from open writer\n" {
		t.Fatalf("open writer after clear: %+v, %v", chunk, err)
	}
	for _, other := range []string{"2026-09-22-000.log", "2026-09-23-001.log"} {
		chunk, err := reader.Read(other, 0, "")
		if err != nil || chunk.Text != "INFO unrelated\n" {
			t.Fatalf("clear affected %s: %+v, %v", other, chunk, err)
		}
	}
	// Clear the active newest segment and continue using the same collector.
	if err := clearer.Clear("2026-09-23-001.log"); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("INFO fresh output\n")); err != nil {
		t.Fatal(err)
	}
	chunk, err = reader.Read("2026-09-23-001.log", 0, "")
	if err != nil || chunk.Text != "INFO fresh output\n" {
		t.Fatalf("collector after clear: %+v, %v", chunk, err)
	}
	if err := clearer.Clear("2026-09-22-000.log"); err != nil {
		t.Fatal(err)
	}
	chunk, err = reader.Read("2026-09-22-000.log", -1, "")
	if err != nil || chunk.Text != "" {
		t.Fatalf("historical clear: %+v, %v", chunk, err)
	}
}

func TestClearInvalidatesCursorsBeforeAndAfterRegrowth(t *testing.T) {
	dataDir := t.TempDir()
	logs, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	writer := logs.Writer()
	if _, err := writer.Write([]byte("INFO old\n")); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	name := files[0].Name
	archive := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + "-000.log"
	if err := os.WriteFile(filepath.Join(logs.dir, archive), []byte("INFO unrelated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unrelated, err := logs.Read(archive, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := logs.Read(name, 0, "")
	if err != nil || before.Generation == "" || before.Reset {
		t.Fatal(before, err)
	}
	if err := logs.Clear(name); err != nil {
		t.Fatal(err)
	}
	unchanged, err := logs.Read(archive, unrelated.NextOffset, unrelated.Generation)
	if err != nil || unchanged.Reset || unchanged.Generation != unrelated.Generation || unchanged.Text != "" {
		t.Fatalf("clear invalidated another file: %+v, %v", unchanged, err)
	}
	empty, err := logs.Read(name, before.NextOffset, before.Generation)
	if err != nil || !empty.Reset || empty.Text != "" || empty.NextOffset != 0 || empty.Generation == before.Generation {
		t.Fatalf("stale cursor after truncate: %+v, %v", empty, err)
	}
	want := strings.Repeat("INFO fresh output\n", 8)
	if _, err := writer.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	after, err := logs.Read(name, before.NextOffset, before.Generation)
	if err != nil || !after.Reset || after.Text != want || after.NextOffset != int64(len(want)) {
		t.Fatalf("stale cursor after regrowth: %+v, %v", after, err)
	}
	if _, err := writer.Write([]byte("INFO later\n")); err != nil {
		t.Fatal(err)
	}
	next, err := logs.Read(name, after.NextOffset, after.Generation)
	if err != nil || next.Reset || next.Text != "INFO later\n" {
		t.Fatalf("resume fresh cursor: %+v, %v", next, err)
	}
	restarted, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	read, err := restarted.Read(name, next.NextOffset, next.Generation)
	if err != nil || !read.Reset || read.Text != want+"INFO later\n" {
		t.Fatalf("cursor after restart: %+v, %v", read, err)
	}
}

func TestClearConcurrentReadersAndWriters(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	line := "INFO appended\n"
	writer := logs.Writer()
	if _, err := writer.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	name := files[0].Name
	cursor, err := logs.Read(name, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	go func() {
		for range 100 {
			if _, err := writer.Write([]byte(line)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	go func() {
		for range 100 {
			if err := logs.Clear(name); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for range 100 {
		cursor, err = logs.Read(name, cursor.NextOffset, cursor.Generation)
		if err != nil || cursor.NextOffset > cursor.Size || strings.ReplaceAll(cursor.Text, line, "") != "" {
			t.Errorf("read during concurrent clear/append: %+v, %v", cursor, err)
			break
		}
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestClearRejectsUnmanagedAndMissingFiles(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "native.log")
	if err := os.WriteFile(outside, []byte("original output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(logs.dir, "2026-09-22-000.log")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(logs.dir, "2026-09-22-001.log"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "../native.log", outside, "2026-09-22-000.log", "2026-09-22-001.log", "2026-09-22-0001.log"} {
		if err := logs.Clear(name); !errors.Is(err, ErrInvalidFile) {
			t.Errorf("clear %q: %v", name, err)
		}
	}
	if err := logs.Clear("2026-09-22-002.log"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "original output" {
		t.Fatalf("native output changed: %q, %v", content, err)
	}
}
