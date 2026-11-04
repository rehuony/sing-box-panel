// SPDX-License-Identifier: GPL-3.0-or-later

package corelogs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFollowCopiesNewOutputOnlyAndFlushesOnClose(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		name := "relative"
		if absolute {
			name = "absolute"
		}
		t.Run(name, func(t *testing.T) {
			logs, _ := New(t.TempDir())
			dir := t.TempDir()
			path := filepath.Join(dir, "native.log")
			if err := os.WriteFile(path, []byte("private preexisting file contents\n"), 0600); err != nil {
				t.Fatal(err)
			}
			outputPath := "native.log"
			if absolute {
				outputPath = path
			}
			config, err := json.Marshal(map[string]any{"log": map[string]any{"output": outputPath}})
			if err != nil {
				t.Fatal(err)
			}
			follower, err := logs.Follow(config, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer follower.Close()
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString("INFO connected\nWARN password=fixture-secret\nERROR last partial line")
			_ = file.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err = follower.Close(); err != nil {
				t.Fatal(err)
			}
			files, _ := logs.List()
			if len(files) != 1 {
				t.Fatal(files)
			}
			chunk, err := logs.Read(files[0].Name, 0, "")
			if err != nil || strings.Contains(chunk.Text, "preexisting") || strings.Contains(chunk.Text, "fixture-secret") || !strings.Contains(chunk.Text, "ERROR last partial line\n") {
				t.Fatalf("chunk=%q err=%v", chunk.Text, err)
			}
			original, _ := os.ReadFile(path)
			if !strings.HasPrefix(string(original), "private preexisting file contents\n") {
				t.Fatal("changed native log file")
			}
		})
	}
}

func TestFollowCreationRotationAndTruncation(t *testing.T) {
	logs, _ := New(t.TempDir())
	dir := t.TempDir()
	follower, err := logs.Follow([]byte(`{"log":{"output":"native.log"}}`), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer follower.Close()
	f := follower.(*fileFollower)
	// Stop its timer before exercising the cursor deterministically.
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "native.log")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
		if err := f.readNew(1 << 20); err != nil {
			t.Fatal(err)
		}
	}
	write("INFO first long log message\n")
	write("INFO truncated\n")
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	write("INFO rotated\n")
	if err := f.readNew(1 << 20); err != nil {
		t.Fatal(err)
	}
	files, _ := logs.List()
	chunk, _ := logs.Read(files[0].Name, 0, "")
	if chunk.Text != "INFO first long log message\nINFO truncated\nINFO rotated\n" {
		t.Fatalf("unexpected output %q", chunk.Text)
	}
}

func TestFollowRejectsSymlinkAndCaptureLoop(t *testing.T) {
	logs, _ := New(t.TempDir())
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	_ = os.WriteFile(secret, []byte("never publish"), 0600)
	_ = os.Symlink(secret, filepath.Join(dir, "link"))
	for _, path := range []string{filepath.Join(dir, "link"), filepath.Join(logs.dir, "2026-01-01-000.log")} {
		config, _ := json.Marshal(map[string]any{"log": map[string]any{"output": path}})
		if f, err := logs.Follow(config, dir); err == nil {
			if f != nil {
				_ = f.Close()
			}
			t.Fatal("accepted unsafe output")
		}
	}
	if f, err := logs.Follow([]byte(`{"log":{"disabled":true,"output":"link"}}`), dir); err != nil || f != nil {
		t.Fatal("disabled log should not open a file")
	}
}
