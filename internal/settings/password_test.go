// SPDX-License-Identifier: GPL-3.0-or-later
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestResetPasswordPreservesSettingsAndPermissions(t *testing.T) {
	path, original := resetFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ResetPassword(t.Context(), path, testutil.ChangedPassword)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var documentBefore, documentAfter map[string]any
	if err := json.Unmarshal(before, &documentBefore); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &documentAfter); err != nil {
		t.Fatal(err)
	}
	documentAfter["auth"].(map[string]any)["password_hash"] = original.Auth.PasswordHash
	if !reflect.DeepEqual(documentBefore, documentAfter) {
		t.Fatal("unrelated settings changed")
	}
	if bytes.Contains(after, []byte(testutil.ChangedPassword)) {
		t.Fatal("plaintext password persisted")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private permissions lost", err)
	}
	if _, err := os.Stat(saved.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reset accessed storage", err)
	}
}

func TestResetPasswordFailureLeavesSettingsUntouched(t *testing.T) {
	for _, failure := range []string{"missing", "invalid", "pending", "canceled", "permission", "symlink"} {
		t.Run(failure, func(t *testing.T) {
			path, _ := resetFixture(t)
			ctx := t.Context()
			switch failure {
			case "missing":
				path = filepath.Join(filepath.Dir(path), "missing", "setting.json")
			case "invalid":
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "pending":
				if err := os.WriteFile(path+".pending", []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses file permissions")
				}
				directory := filepath.Dir(path)
				if err := os.Chmod(directory, 0500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(directory, 0700)
			case "symlink":
				target := path
				path += ".link"
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(path)
			if _, err := ResetPassword(ctx, path, testutil.ChangedPassword); err == nil {
				t.Fatal("invalid reset succeeded")
			}
			after, err := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("failed reset changed settings")
			}
			if failure == "missing" && !errors.Is(err, os.ErrNotExist) {
				t.Fatal("missing config was created", err)
			}
		})
	}
}

func TestPasswordResetAndOtherSettingsWritesSerialize(t *testing.T) {
	path, _ := resetFixture(t)
	start := make(chan struct{})
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Go(func() { <-start; _, err := ResetPassword(t.Context(), path, testutil.ChangedPassword); failures <- err })
	workers.Go(func() { <-start; failures <- ResetFields(t.Context(), path, []string{"server.port"}) })
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	after, err := Load(path)
	if err != nil || after.Server.Port != Defaults().Server.Port || after.Auth.PasswordHash == testutil.PasswordHash {
		t.Fatal("concurrent write lost an update", err)
	}
}
