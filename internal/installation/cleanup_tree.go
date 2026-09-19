// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// cleanupTree holds each directory's ownership locks from enumeration through
// removal. Only the selected settings' ancestors stay open until finalization;
// ordinary branches release their descriptors as traversal unwinds.
type cleanupTree struct {
	ctx          context.Context
	dataDir      string
	settings     string // Relative to dataDir; empty when settings are outside it.
	result       *CleanupResult
	settingsDirs []*cleanupDirectory // Deepest first, still locked and open.
}

type cleanupDirectory struct {
	parent *os.Root
	root   *os.Root
	lock   *os.File
	lease  *panelprocess.Lease
	info   fs.FileInfo
	name   string
	path   string
}

func openCleanupDirectory(parent *os.Root, name, path string, expected fs.FileInfo) (_ *cleanupDirectory, err error) {
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	directory := &cleanupDirectory{parent: parent, root: root, info: expected, name: name, path: path}
	defer func() {
		if err != nil {
			err = errors.Join(err, directory.Close())
		}
	}()
	directory.lock, err = store.LockDirectoryForCleanup(root.Name())
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	locked, err := directory.lock.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(expected, opened) || !os.SameFile(opened, locked) {
		return nil, fmt.Errorf("directory changed while acquiring cleanup locks: %s", path)
	}
	// A runtime may still own its lease during startup or shutdown, without an
	// open database. Do not create lease files in ordinary content directories.
	leaseInfo, err := root.Lstat(panelprocess.LeaseFileName)
	if err == nil && leaseInfo.Mode().IsRegular() {
		directory.lease, err = panelprocess.AcquireLease(root.Name())
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return directory, nil
}

func (directory *cleanupDirectory) Close() error {
	var err error
	if directory.lease != nil {
		err = directory.lease.Close()
	}
	if directory.lock != nil {
		err = errors.Join(err, directory.lock.Close())
	}
	return errors.Join(err, directory.root.Close())
}

func (directory *cleanupDirectory) remove() error {
	current, err := directory.parent.Lstat(directory.name)
	if err != nil {
		return err
	}
	if !os.SameFile(directory.info, current) {
		return fmt.Errorf("directory changed before removal: %s", directory.path)
	}
	// Never recurse here: entries created after enumeration must cause an error,
	// rather than being removed without their own ownership checks.
	return directory.parent.Remove(directory.name)
}

func (tree *cleanupTree) remove(parent *os.Root, name, path string) (removeErr error) {
	if err := tree.ctx.Err(); err != nil {
		return err
	}
	if tree.settings != "" && (path == tree.settings || path == tree.settings+".lock" || path == tree.settings+".location") {
		return nil
	}
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		directory, err := openCleanupDirectory(parent, name, path, info)
		if err != nil {
			return fmt.Errorf("lock cleanup directory %s: %w", path, err)
		}
		retained := false
		defer func() {
			if !retained {
				removeErr = errors.Join(removeErr, directory.Close())
			}
		}()
		children, err := fs.ReadDir(directory.root.FS(), ".")
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := tree.remove(directory.root, child.Name(), filepath.Join(path, child.Name())); err != nil {
				return err
			}
		}
		if tree.settings != "" && pathWithin(path, tree.settings) {
			tree.settingsDirs = append(tree.settingsDirs, directory)
			retained = true
			return nil
		}
		err = directory.remove()
		if err != nil {
			return fmt.Errorf("remove instance directory %s: %w", path, err)
		}
	} else if err := parent.Remove(name); err != nil {
		return fmt.Errorf("remove instance path %s: %w", path, err)
	}
	tree.result.Removed = append(tree.result.Removed, filepath.Join(tree.dataDir, path))
	return nil
}

func (tree *cleanupTree) removeSettingsDirectories() error {
	for _, directory := range tree.settingsDirs {
		if err := tree.ctx.Err(); err != nil {
			return err
		}
		if err := directory.remove(); err != nil {
			return fmt.Errorf("remove settings directory %s: %w", directory.path, err)
		}
		tree.result.Removed = append(tree.result.Removed, filepath.Join(tree.dataDir, directory.path))
	}
	return nil
}

func (tree *cleanupTree) Close() error {
	var err error
	for _, directory := range tree.settingsDirs {
		err = errors.Join(err, directory.Close())
	}
	return err
}
