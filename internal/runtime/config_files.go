// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// startupConfigFiles owns disposable execution files, not durable history. Its
// separate lock lets a reaper release a file while Stop holds operationMu and
// waits for that reaper. Materialization and last-user removal are serialized.
type startupConfigFiles struct {
	mu         sync.Mutex
	runtimeDir string
	users      map[string]int
}

type startupConfigFile struct {
	path  string
	files *startupConfigFiles
	once  sync.Once
	err   error
}

func (files *startupConfigFiles) acquire(digest coreartifact.SHA256, data []byte) (*startupConfigFile, error) {
	files.mu.Lock()
	defer files.mu.Unlock()
	path, err := materializeStartupConfig(files.runtimeDir, digest, data)
	if err != nil {
		return nil, err
	}
	files.users[filepath.Base(path)]++
	return &startupConfigFile{path: path, files: files}, nil
}

func (file *startupConfigFile) Close() error {
	file.once.Do(func() {
		file.files.mu.Lock()
		defer file.files.mu.Unlock()
		file.files.users[filepath.Base(file.path)]--
		file.err = file.files.removeUnusedLocked()
	})
	return file.err
}

// prune also discovers files left by interrupted panel operations. Call only
// after startup reconciliation proves that no unowned core may still use them.
func (files *startupConfigFiles) prune(ctx context.Context) error {
	files.mu.Lock()
	defer files.mu.Unlock()
	root, err := openStartupConfigDirectory(files.runtimeDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !managedStartupConfigName(name) || !entry.Type().IsRegular() {
			continue
		}
		if _, exists := files.users[name]; !exists {
			files.users[name] = 0
		}
	}
	return files.removeUnusedLocked()
}

// Keep failed removals at zero users so the next release or prune retries them.
// No caller reports cleanup failure as a failed check or process transition.
func (files *startupConfigFiles) removeUnusedLocked() error {
	root, err := openStartupConfigDirectory(files.runtimeDir)
	if errors.Is(err, os.ErrNotExist) {
		for name, users := range files.users {
			if users == 0 {
				delete(files.users, name)
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	var cleanupErr error
	for name, users := range files.users {
		if users != 0 {
			continue
		}
		info, err := root.Lstat(name)
		if err == nil {
			if !info.Mode().IsRegular() {
				err = errors.New("startup config cleanup refuses a non-regular file")
			} else {
				err = root.Remove(name)
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("clean startup config %s: %w", name, err))
			continue
		}
		delete(files.users, name)
	}
	return cleanupErr
}

func managedStartupConfigName(name string) bool {
	if digest, ok := strings.CutSuffix(name, ".json"); ok && len(digest) == 64 && strings.ToLower(digest) == digest {
		_, err := coreartifact.ParseSHA256(digest)
		return err == nil
	}
	// os.CreateTemp appends a decimal random suffix to this fixed prefix.
	suffix, ok := strings.CutPrefix(name, ".startup-config-")
	if !ok || len(suffix) == 0 || len(suffix) > 10 {
		return false
	}
	for _, character := range suffix {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// Open relative to validated directory handles. Cleanup never follows the
// runtime/configs symlink or escapes into a replacement parent directory.
func openStartupConfigDirectory(runtimeDir string) (*os.Root, error) {
	info, err := os.Lstat(runtimeDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("runtime directory is not private")
	}
	runtimeRoot, err := os.OpenRoot(runtimeDir)
	if err != nil {
		return nil, err
	}
	defer runtimeRoot.Close()
	opened, err := runtimeRoot.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("runtime directory changed while opening"), err)
	}
	info, err = runtimeRoot.Lstat("configs")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("runtime config directory is not private")
	}
	root, err := runtimeRoot.OpenRoot("configs")
	if err != nil {
		return nil, err
	}
	opened, err = root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, errors.Join(errors.New("runtime config directory changed while opening"), err)
	}
	return root, nil
}

func (manager *Manager) releaseStartupConfig(file *startupConfigFile) {
	if err := file.Close(); err != nil {
		manager.reportConfigCleanupError(err)
	}
}

func (manager *Manager) reportConfigCleanupError(err error) {
	if manager.options.ObserveConfigCleanupError != nil {
		manager.options.ObserveConfigCleanupError(err)
	}
}

// PruneStartupConfigs removes interrupted-operation files, preserving all files
// held by checks or owned processes. The server must first reconcile old process
// observations while holding the exclusive runtime lease.
func (manager *Manager) PruneStartupConfigs(ctx context.Context) {
	if err := manager.configs.prune(ctx); err != nil {
		manager.reportConfigCleanupError(err)
	}
}
