// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeStartupConfigIsExactPrivateAndIdempotent(t *testing.T) {
	t.Parallel()

	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	config := []byte("// preserved comment\n{\n  \"route\": {}\n}\n")
	digest := digestOf(config)
	first, err := materializeStartupConfig(runtimeDirectory, digest, config)
	if err != nil {
		t.Fatalf("materializeStartupConfig(first): %v", err)
	}
	second, err := materializeStartupConfig(runtimeDirectory, digest, config)
	if err != nil {
		t.Fatalf("materializeStartupConfig(second): %v", err)
	}
	if first != second {
		t.Fatalf("paths differ: %q != %q", first, second)
	}
	actual, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(actual) != string(config) {
		t.Fatalf("config bytes = %q, want exact %q", actual, config)
	}
	info, err := os.Lstat(first)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want regular 0600", info.Mode())
	}
}

func TestMaterializeStartupConfigRejectsExistingCorruption(t *testing.T) {
	t.Parallel()

	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	config := []byte(`{"route":{}}`)
	digest := digestOf(config)
	path, err := materializeStartupConfig(runtimeDirectory, digest, config)
	if err != nil {
		t.Fatalf("materializeStartupConfig: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"route":{"mutated":true}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}
	if _, err := materializeStartupConfig(runtimeDirectory, digest, config); !errors.Is(err, ErrMaterialization) {
		t.Fatalf("materialize corrupted config error = %v, want ErrMaterialization", err)
	}
}

func TestMaterializeStartupConfigRejectsSymlinkTarget(t *testing.T) {
	t.Parallel()

	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	configDirectory := filepath.Join(runtimeDirectory, "configs")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	config := []byte(`{"route":{}}`)
	finalPath := filepath.Join(configDirectory, digestOf(config).String()+".json")
	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, config, 0o600); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}
	if err := os.Symlink(target, finalPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := materializeStartupConfig(runtimeDirectory, digestOf(config), config); !errors.Is(err, ErrMaterialization) {
		t.Fatalf("materialize symlink error = %v, want ErrMaterialization", err)
	}
}

func TestMaterializeStartupConfigRejectsNonPrivateRuntimeDirectory(t *testing.T) {
	t.Parallel()

	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDirectory, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	config := []byte(`{"route":{}}`)
	if _, err := materializeStartupConfig(runtimeDirectory, digestOf(config), config); !errors.Is(err, ErrMaterialization) {
		t.Fatalf("materialize unsafe directory error = %v, want ErrMaterialization", err)
	}
}
