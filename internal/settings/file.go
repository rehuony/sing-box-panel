// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Read returns the selected settings file unchanged, including invalid JSON.
func Read(path string) ([]byte, error) {
	if err := CheckPending(path); err != nil {
		return nil, err
	}
	data, err := ReadRaw(path)
	if err != nil {
		return nil, err
	}
	if err := CheckPending(path); err != nil {
		return nil, err
	}
	return data, nil
}

// ReadRaw is reserved for startup recovery while a file transaction is pending.
func ReadRaw(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read settings %q: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read settings %q: %w", path, err)
	}
	if len(data) > MaximumBytes {
		return nil, fmt.Errorf("settings %q exceeds %d bytes", path, MaximumBytes)
	}
	return data, nil
}

// Replace validates and atomically saves a complete settings document. Relative
// paths resolve against the destination; no data directory or database is opened.
func Replace(path string, data []byte) error {
	return ReplaceContext(context.Background(), path, data)
}

func ReplaceContext(ctx context.Context, path string, data []byte) error {
	if _, err := parse(path, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock, err := Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := CheckPending(path); err != nil {
		return err
	}
	if err := rememberDataLocationBeforeReplace(path); err != nil {
		return err
	}
	return ReplaceLocked(path, data)
}

func rememberDataLocationBeforeReplace(path string) error {
	// Remember the old location before publishing a new data_dir. This does not
	// open or mutate either data directory.
	if old, err := ConfiguredDataDir(path); err == nil {
		if err := RememberDataLocation(path, old); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		// A damaged file can still be replaced; an existing location record, if
		// any, remains authoritative for migration recovery.
		if _, stateErr := ReadDataLocation(path); stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
			return stateErr
		}
	}
	return nil
}

// ReplaceLocked requires the caller to hold Lock and coordinate any pending journal.
func ReplaceLocked(path string, data []byte) error {
	if _, err := parse(path, data); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("settings destination %q must be a regular file", path)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect settings %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	return atomicWrite(path, data, 0o600, true)
}

var ErrPending = errors.New("panel settings update needs recovery; start the panel before editing settings")

func CheckPending(path string) error {
	if _, err := os.Lstat(path + ".pending"); err == nil {
		return ErrPending
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Revision is an opaque, JSON-safe fingerprint. Formatting and manual edits
// change it too; callers must not interpret it as a sequence number.
func Revision(data []byte) int64 {
	digest := sha256.Sum256(data)
	return int64(binary.BigEndian.Uint64(digest[:8]) & ((1 << 53) - 1))
}

// WriteAtomic publishes recovery metadata with the same durability as settings.
func WriteAtomic(path string, data []byte) error { return atomicWrite(path, data, 0600, true) }

func RemoveDurable(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
