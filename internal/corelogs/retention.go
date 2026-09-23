// SPDX-License-Identifier: GPL-3.0-or-later
package corelogs

import (
	"errors"
	"os"
)

// SetPolicy updates the writer and enforces retention without truncating files.
// A cleanup error does not undo the new policy; the next cleanup retries it.
func (f *Files) SetPolicy(policy Policy) error {
	if policy.RetentionDays < 1 || policy.RetentionDays > 3650 || policy.MaxFiles < 0 || policy.MaxFiles > 1024 || policy.MaxFileBytes < 1<<20 || policy.MaxFileBytes > 1024<<20 {
		return errors.New("invalid core log policy")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.policy = policy
	return f.pruneLocked()
}

func (f *Files) Prune() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pruneLocked()
}

func (f *Files) pruneLocked() error {
	files, err := f.List()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	cutoff := f.now().UTC().AddDate(0, 0, 1-f.policy.RetentionDays).Format("2006-01-02")
	remaining := len(files)
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.Name == f.active && file.Name[:10] >= cutoff {
			continue
		}
		if file.Name[:10] >= cutoff && (f.policy.MaxFiles == 0 || remaining <= f.policy.MaxFiles) {
			continue
		}
		info, err := root.Lstat(file.Name)
		if errors.Is(err, os.ErrNotExist) {
			remaining--
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := root.Remove(file.Name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		remaining--
	}
	return nil
}
