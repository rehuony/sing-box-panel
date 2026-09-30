// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCollectGoModulesPreservesCachePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go command wrapper requires a POSIX shell")
	}
	goCommand, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, override := range []bool{false, true} {
		name := "saved Go configuration"
		if override {
			name = "environment overrides saved configuration"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			want := make(map[string]string)
			var config strings.Builder
			for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE"} {
				path := filepath.Join(root, "saved paths", key)
				config.WriteString(key + "=" + path + "\n")
				want[key] = path
				t.Setenv(key, "")
				if override {
					want[key] = filepath.Join(root, "environment paths", key)
					t.Setenv(key, want[key])
				}
			}
			configPath := filepath.Join(root, "go.env")
			if err := os.WriteFile(configPath, []byte(config.String()), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GOENV", configPath)
			t.Setenv("GOTOOLCHAIN", "local")
			t.Setenv("NOTICES_TEST_GO", goCommand)
			t.Setenv("NOTICES_TEST_ENV_DIR", root)

			// Let the real Go tool resolve settings, but replace dependency loading
			// with an environment capture so this test never downloads modules.
			wrapper := `#!/bin/sh
if [ "$1" = list ]; then
  exec "$NOTICES_TEST_GO" env -json GOPATH GOMODCACHE GOCACHE GOENV > "$NOTICES_TEST_ENV_DIR/$GOARCH.json"
fi
exec "$NOTICES_TEST_GO" "$@"
`
			if err := os.WriteFile(filepath.Join(root, "go"), []byte(wrapper), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			if _, err := collectGoModules(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			want["GOENV"] = "" // go env reports no configuration file when GOENV=off.
			for _, architecture := range []string{"amd64", "arm64"} {
				output, err := os.ReadFile(filepath.Join(root, architecture+".json"))
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]string
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatal(err)
				}
				for key, value := range want {
					if got[key] != value {
						t.Errorf("linux/%s %s = %q, want %q", architecture, key, got[key], value)
					}
				}
			}
		})
	}
}
