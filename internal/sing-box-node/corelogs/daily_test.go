// SPDX-License-Identifier: GPL-3.0-or-later

package corelogs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureCurrentFileRotatesIdleUTCDaysAndRetainsEmptyFiles(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 7, 59, 59, 0, time.FixedZone("UTC+8", 8*60*60))
	logs.now = func() time.Time { return now }
	policy := DefaultPolicy()
	policy.RetentionDays = 2
	if err := logs.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := logs.EnsureCurrentFile(); err != nil {
			t.Fatal(err)
		}
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 || files[0].Name != "2026-09-25-000.log" || files[0].Size != 0 || files[0].Deletable {
		t.Fatalf("initial empty UTC file: %+v, %v", files, err)
	}
	output := logs.Writer()
	if _, err := output.Write([]byte("INFO before midnight\n")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err := logs.EnsureCurrentFile(); err != nil {
		t.Fatal(err)
	}
	files, err = logs.List()
	if err != nil || len(files) != 2 || files[0].Name != "2026-09-26-000.log" || files[0].Size != 0 || !files[1].Deletable {
		t.Fatalf("idle midnight rotation: %+v, %v", files, err)
	}
	previous, err := logs.Read("2026-09-25-000.log", 0, "")
	if err != nil || previous.Text != "INFO before midnight\n" {
		t.Fatalf("previous day changed: %+v, %v", previous, err)
	}
	now = now.AddDate(0, 0, 1)
	if err := logs.EnsureCurrentFile(); err != nil {
		t.Fatal(err)
	}
	files, err = logs.List()
	if err != nil || len(files) != 2 || files[0].Name != "2026-09-27-000.log" || files[1].Name != "2026-09-26-000.log" || files[1].Size != 0 {
		t.Fatalf("empty file retention: %+v, %v", files, err)
	}
	empty, err := logs.Read(files[0].Name, 0, "")
	if err != nil || empty.Text != "" || empty.NextOffset != 0 {
		t.Fatalf("empty file cursor: %+v, %v", empty, err)
	}
	if _, err := output.Write([]byte("INFO first new output\n")); err != nil {
		t.Fatal(err)
	}
	chunk, err := logs.Read(empty.File, empty.NextOffset, empty.Generation)
	if err != nil || chunk.Text != "INFO first new output\n" || chunk.Reset {
		t.Fatalf("existing writer/cursor after idle rotation: %+v, %v", chunk, err)
	}
}

func TestEnsureCurrentFileReusesNewestSegmentAfterRestartAndPolicyChange(t *testing.T) {
	dataDir := t.TempDir()
	logs, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	name := time.Now().UTC().Format("2006-01-02") + "-005.log"
	fullLogFile(t, logs.dir, name)
	logs, err = New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	policy := DefaultPolicy()
	policy.MaxFileBytes = 1 << 20
	policy.MaxFiles = 1
	if err := logs.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := logs.EnsureCurrentFile(); err != nil {
			t.Fatal(err)
		}
	}
	files, err := logs.List()
	if err != nil || len(files) != 1 || files[0].Name != name || files[0].Size != DefaultPolicy().MaxFileBytes {
		t.Fatalf("maintenance rotated/truncated an existing segment: %+v, %v", files, err)
	}
	if _, err := logs.Writer().Write([]byte("INFO size rotation\n")); err != nil {
		t.Fatal(err)
	}
	files, err = logs.List()
	if err != nil || len(files) != 1 || files[0].Name != name[:11]+"006.log" {
		t.Fatalf("write must still rotate and enforce count retention: %+v, %v", files, err)
	}
}

func TestEnsureCurrentFileRejectsUnsafeDestination(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "native.log")
	if err := os.WriteFile(outside, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	name := time.Now().UTC().Format("2006-01-02") + "-000.log"
	if err := os.Symlink(outside, filepath.Join(logs.dir, name)); err != nil {
		t.Fatal(err)
	}
	if err := logs.EnsureCurrentFile(); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("unsafe daily file accepted: %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "original" {
		t.Fatalf("native file changed: %q, %v", data, err)
	}
}
