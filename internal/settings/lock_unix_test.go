// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRootReplacementPreservesServiceOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to assign the service owner")
	}
	path := filepath.Join(t.TempDir(), "setting.json")
	value := Defaults()
	value.Auth.Token = "fixture-token"
	raw, _ := json.Marshal(value)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	value.Panel.Language = "en"
	raw, _ = json.Marshal(value)
	if err := Replace(path, raw); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + ".lock"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		stat := info.Sys().(*syscall.Stat_t)
		if stat.Uid != 65534 || stat.Gid != 65534 || info.Mode().Perm() != 0600 {
			t.Fatalf("replacement lost service ownership or private mode: uid=%d gid=%d mode=%o", stat.Uid, stat.Gid, info.Mode().Perm())
		}
	}
}

func TestDefaultDataDirectoryKeepsUserAndRootLocations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	want := filepath.Join(home, ".local", "share", "sing-box-panel")
	if os.Geteuid() == 0 {
		want = "/var/lib/sing-box-panel"
	}
	if got := Defaults().DataDir; got != want {
		t.Fatalf("data directory = %q, want %q", got, want)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "custom"))
	if os.Geteuid() != 0 {
		want = filepath.Join(home, "custom", "sing-box-panel")
	}
	if got := Defaults().DataDir; got != want {
		t.Fatalf("XDG data directory = %q, want %q", got, want)
	}
}
