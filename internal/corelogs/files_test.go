// SPDX-License-Identifier: GPL-3.0-or-later
package corelogs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotationRetainsNewOutputAfterPruningAndReopening(t *testing.T) {
	dataDir := t.TempDir()
	logs, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	day := now.Format("2006-01-02")
	for index := range maxFiles {
		fullLogFile(t, logs.dir, fmt.Sprintf("%s-%03d.log", day, index))
	}
	output := logs.Writer()
	for index := range 3 {
		if _, err := fmt.Fprintf(output, "INFO after rotation %d\n", index); err != nil {
			t.Fatal(err)
		}
	}
	logs, err = New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	logs.now = func() time.Time { return now }
	if _, err := logs.Writer().Write([]byte("INFO after reopening\n")); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != maxFiles {
		t.Fatalf("retained files: count=%d error=%v", len(files), err)
	}
	chunk, err := logs.Read(files[0].Name, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "INFO after rotation 0\nINFO after rotation 1\nINFO after rotation 2\nINFO after reopening\n"
	if chunk.Text != want {
		t.Fatalf("new output was lost during retention: got %q, want %q", chunk.Text, want)
	}
}

func TestOneWriterContinuesAcrossUTCDateChange(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 19, 23, 59, 59, 0, time.UTC)
	logs.now = func() time.Time { return now }
	output := logs.Writer()
	if _, err := output.Write([]byte("INFO before midnight\nINFO split")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := output.Write([]byte(" line\nINFO after midnight\n")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"2026-09-19-000.log": "INFO before midnight\n",
		"2026-09-20-000.log": "INFO split line\nINFO after midnight\n",
	} {
		chunk, err := logs.Read(name, 0)
		if err != nil || chunk.Text != want {
			t.Fatalf("output for %s: got %q, want %q; error=%v", name, chunk.Text, want, err)
		}
	}
}

func TestRotationContinuesBeyondThreeDigitSequence(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	fullLogFile(t, logs.dir, "2026-09-19-999.log")
	output := logs.Writer()
	for range 2 {
		if _, err := output.Write([]byte("INFO continued\n")); err != nil {
			t.Fatal(err)
		}
	}
	files, err := logs.List()
	if err != nil || len(files) != 2 || files[0].Name != "2026-09-19-1000.log" {
		t.Fatalf("numeric rotation order: %+v, %v", files, err)
	}
	chunk, err := logs.Read(files[0].Name, 0)
	if err != nil || chunk.Text != "INFO continued\nINFO continued\n" {
		t.Fatalf("continued output: %+v, %v", chunk, err)
	}
}

func TestRetentionKeepsActiveFileAfterClockMovesBackwards(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	for index := range maxFiles {
		fullLogFile(t, logs.dir, fmt.Sprintf("2026-09-20-%03d.log", index))
	}
	if _, err := logs.Writer().Write([]byte("INFO current output\n")); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != maxFiles {
		t.Fatalf("retention count: %d, %v", len(files), err)
	}
	chunk, err := logs.Read("2026-09-19-000.log", 0)
	if err != nil || chunk.Text != "INFO current output\n" {
		t.Fatalf("active output: %+v, %v", chunk, err)
	}
}

func fullLogFile(t *testing.T, dir, name string) {
	t.Helper()
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxFileBytes); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCapturedOutputAndCursor(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := logs.Writer()
	for _, text := range []string{"\x1b[32mINFO hello", " world\x1b[0m\nWARN password=", "sensitive\n"} {
		if _, err = out.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	chunk, err := logs.Read(files[0].Name, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(chunk.Text, "INFO hello world\n") || strings.Contains(chunk.Text, "sensitive") || strings.Contains(chunk.Text, "\x1b") {
		t.Fatal(chunk.Text)
	}
	_, _ = out.Write([]byte("ERROR EOF\n"))
	next, err := logs.Read(files[0].Name, chunk.NextOffset)
	if err != nil || next.Text != "ERROR EOF\n" {
		t.Fatal(next, err)
	}
	if _, err = logs.Read("../../secret", 0); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestSymlinkAndOversizedOutput(t *testing.T) {
	logs, _ := New(t.TempDir())
	outside := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(outside, []byte("secret"), 0600)
	name := "2026-01-01-000.log"
	if err := os.Symlink(outside, filepath.Join(logs.dir, name)); err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Read(name, 0); err == nil {
		t.Fatal("read symlink")
	}
	out := logs.Writer()
	_, err := out.Write([]byte(strings.Repeat("a", 100_000) + "\nINFO recovered\n"))
	if err != nil {
		t.Fatal(err)
	}
	files, _ := logs.List()
	chunk, err := logs.Read(files[0].Name, -1)
	if err != nil || !strings.Contains(chunk.Text, "INFO recovered") || len(chunk.Text) > 200 {
		t.Fatal(chunk, err)
	}
}

func TestCursorChunksPreserveUnicodeAcrossBoundaries(t *testing.T) {
	logs, _ := New(t.TempDir())
	line := "INFO " + strings.Repeat("节点", 500) + "\n"
	want := strings.Repeat(line, 40)
	if _, err := logs.Writer().Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	files, _ := logs.List()
	offset := int64(0)
	got := ""
	for offset < files[0].Size {
		chunk, err := logs.Read(files[0].Name, offset)
		if err != nil || chunk.NextOffset <= offset {
			t.Fatal(chunk, err)
		}
		got += chunk.Text
		offset = chunk.NextOffset
	}
	if got != want {
		t.Fatalf("Unicode output did not round trip: got %d bytes want %d", len(got), len(want))
	}
}

func TestFlushSeparatesProcessOutputAndRedactsPartialLine(t *testing.T) {
	logs, _ := New(t.TempDir())
	out := logs.Writer()
	_, _ = out.Write([]byte("ERROR password=secret"))
	if err := out.(interface{ Flush() error }).Flush(); err != nil {
		t.Fatal(err)
	}
	_, _ = out.Write([]byte("INFO restarted\n"))
	files, _ := logs.List()
	chunk, err := logs.Read(files[0].Name, -1)
	if err != nil || strings.Contains(chunk.Text, "secret") || !strings.Contains(chunk.Text, "\nINFO restarted\n") {
		t.Fatal(chunk, err)
	}
}
