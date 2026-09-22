// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentLinkSwitchAndValidation(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := New(Options{Root: filepath.Join(root, "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(artifacts.root, "current")
	var previous string
	for _, content := range []string{"first binary", "second binary"} {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		binary := filepath.Join(artifacts.root, "sha256", digest[:2], digest, "sing-box")
		if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binary, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
		if err := artifacts.SetCurrent(ctx, filepath.Dir(binary)); err == nil {
			t.Fatal("accepted directory instead of executable")
		}
		if previous != "" {
			if target, err := filepath.EvalSymlinks(current); err != nil || target != previous {
				t.Fatalf("failed selection changed link: %q %v", target, err)
			}
		}
		if err := artifacts.SetCurrent(ctx, binary); err != nil {
			t.Fatal(err)
		}
		if err := artifacts.SetCurrent(ctx, binary); err != nil {
			t.Fatal(err)
		}
		if target, err := filepath.EvalSymlinks(current); err != nil || target != binary {
			t.Fatalf("selected link: %q %v", target, err)
		}
		if target, err := os.Readlink(current); err != nil || filepath.IsAbs(target) {
			t.Fatalf("link must be relative: %q %v", target, err)
		}
		previous = binary
	}
	if err := artifacts.SetCurrent(ctx, "/outside/sing-box"); err == nil {
		t.Fatal("accepted external binary")
	}
	if err := artifacts.SetCurrent(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(current); !os.IsNotExist(err) {
		t.Fatalf("link remains: %v", err)
	}
	if err := os.WriteFile(current, []byte("operator file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.SetCurrent(ctx, ""); err == nil {
		t.Fatal("removed an unrelated regular file")
	}
}
