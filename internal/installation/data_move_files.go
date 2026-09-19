// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func lockDataMoveTree(ctx context.Context, root string) ([]*os.File, error) {
	var locks []*os.File
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			lock, err := store.LockDirectoryForCleanup(path)
			if err != nil {
				return err
			}
			locks = append(locks, lock)
		} else if filepath.Dir(path) != root && (entry.Name() == "panel.db" || entry.Name() == panelprocess.LeaseFileName) {
			return errors.New("data directory contains another panel instance; move it separately first")
		}
		return nil
	})
	if err != nil {
		closeDataMoveLocks(locks)
		return nil, err
	}
	return locks, nil
}

func closeDataMoveLocks(locks []*os.File) {
	for _, lock := range locks {
		lock.Close()
	}
}

func copyDataMoveTree(ctx context.Context, source, target string) error {
	// Only an owned, unpublished destination reaches this point. Rebuild partial
	// copies after interruption instead of trusting an unfinished SQLite snapshot.
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == dataMoveMarker || entry.Name() == panelprocess.LeaseFileName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if filepath.Dir(rel) == "." {
			switch rel {
			case "panel.db", "panel.db-wal", "panel.db-shm", "panel-control.sock", panelprocess.LeaseFileName, dataMoveMarker:
				return nil
			}
		}
		destination := filepath.Join(target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.Mkdir(destination, 0700)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(link) && pathWithin(source, link) {
				rel, _ := filepath.Rel(source, link)
				link = filepath.Join(target, rel)
			}
			return os.Symlink(link, destination)
		case info.Mode().IsRegular():
			return copyDataMoveFile(ctx, path, destination, info)
		default:
			return fmt.Errorf("unsupported data entry during migration: %s", rel)
		}
	})
}

type dataMoveReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r dataMoveReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

func copyDataMoveFile(ctx context.Context, source, destination string, expected fs.FileInfo) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil || !os.SameFile(expected, opened) {
		return errors.New("source changed during migration")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, output.Close()) }()
	size, err := io.Copy(output, dataMoveReader{ctx: ctx, reader: input})
	if err != nil {
		return err
	}
	after, err := input.Stat()
	if err != nil || size != expected.Size() || after.Size() != expected.Size() || after.ModTime() != expected.ModTime() {
		return errors.New("source changed while copying data")
	}
	if err := output.Chmod(expected.Mode().Perm()); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	return nil
}

// Check every remaining source entry before removal, including after a crash
// in the cleanup phase. A changed or missing destination preserves the source.
func verifyDataMoveFiles(ctx context.Context, source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." || entry.IsDir() {
			return nil
		}
		if filepath.Dir(rel) == "." {
			switch rel {
			case "panel.db", "panel.db-wal", "panel.db-shm", "panel-control.sock", panelprocess.LeaseFileName, dataMoveMarker:
				return nil
			}
		}
		destination := filepath.Join(target, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			original, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(original) && pathWithin(source, original) {
				link, _ := filepath.Rel(source, original)
				original = filepath.Join(target, link)
			}
			copied, err := os.Readlink(destination)
			if err != nil || original != copied {
				return errors.New("destination link changed during migration")
			}
			return nil
		}
		var digests [2][32]byte
		for i, name := range []string{path, destination} {
			info, err := os.Lstat(name)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return errors.New("migration file changed type")
			}
			file, err := os.Open(name)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, err = io.Copy(hash, dataMoveReader{ctx: ctx, reader: file})
			err = errors.Join(err, file.Close())
			if err != nil {
				return err
			}
			copy(digests[i][:], hash.Sum(nil))
		}
		if digests[0] != digests[1] {
			return fmt.Errorf("copied data differs at %s; source retained", rel)
		}
		return nil
	})
}

func syncDataMoveTree(ctx context.Context, root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// MkdirAll may also have created ancestors of the destination. Persist their
	// entries before the source is removed, not just the copied tree itself.
	for parent := filepath.Dir(root); ; parent = filepath.Dir(parent) {
		file, err := os.Open(parent)
		if err != nil {
			return err
		}
		err = errors.Join(file.Sync(), file.Close())
		if err != nil {
			return err
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	for i := len(directories) - 1; i >= 0; i-- {
		file, err := os.Open(directories[i])
		if err != nil {
			return err
		}
		err = errors.Join(file.Sync(), file.Close())
		if err != nil {
			return err
		}
	}
	return nil
}

func removeMovedSource(ctx context.Context, source string, expected fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := os.Lstat(source)
	if err != nil || !os.SameFile(expected, current) || !current.IsDir() {
		return errors.New("source directory changed before migration completed")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == dataMoveMarker || entry.Name() == panelprocess.LeaseFileName {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(source, entry.Name())); err != nil {
			return err
		}
	}
	// A restrictive service sandbox may allow removing the contents but not the
	// old directory itself. Its private marker then prevents reuse as an empty
	// instance; all product data already resides at the verified destination.
	for _, name := range []string{panelprocess.LeaseFileName} {
		if err := os.Remove(filepath.Join(source, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	marker, err := readDataMoveOwner(source)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(source, dataMoveMarker)); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		if restoreErr := writeDataMoveOwner(source, marker); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		// Only an empty directory or our marker may remain; do not conceal new data.
		entries, readErr := os.ReadDir(source)
		if readErr != nil || len(entries) != 1 || entries[0].Name() != dataMoveMarker {
			return errors.Join(err, readErr)
		}
		return nil
	}
	parent, err := os.Open(filepath.Dir(source))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}
