// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Inspect credentials directly instead of relying on userdel's busy check,
// which can skip processes whose /proc/PID/root is inaccessible. Check every
// thread, including saved/filesystem IDs and supplementary groups.
func (manager *Manager) requireAccountIdle(ctx context.Context, account systemAccount) error {
	processes, err := os.ReadDir(manager.procRoot)
	if err != nil {
		return fmt.Errorf("inspect processes before account removal: %w", err)
	}
	inspected := 0
	for _, process := range processes {
		if !validServiceID(process.Name()) {
			continue
		}
		taskRoot := filepath.Join(manager.procRoot, process.Name(), "task")
		threads, err := os.ReadDir(taskRoot)
		if errors.Is(err, os.ErrNotExist) {
			continue // The process exited during inspection.
		}
		if err != nil {
			return fmt.Errorf("inspect process %s: %w", process.Name(), err)
		}
		for _, thread := range threads {
			if !validServiceID(thread.Name()) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			status, err := os.ReadFile(filepath.Join(taskRoot, thread.Name(), "status"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect thread %s: %w", thread.Name(), err)
			}
			busy, err := accountUsesCredentials(string(status), account)
			if err != nil {
				return fmt.Errorf("inspect thread %s: %w", thread.Name(), err)
			}
			inspected++
			if busy {
				return accountConflict(fmt.Sprintf("service UID or GID is still used by process %s (thread %s); stop it before uninstalling", process.Name(), thread.Name()))
			}
		}
	}
	if inspected == 0 {
		return errors.New("no process credentials could be inspected before account removal")
	}
	return ctx.Err()
}

func accountUsesCredentials(status string, account systemAccount) (bool, error) {
	seen := make(map[string]bool, 3)
	busy := false
	for _, line := range strings.Split(status, "\n") {
		key, value, _ := strings.Cut(line, ":")
		if key != "Uid" && key != "Gid" && key != "Groups" {
			continue
		}
		ids := strings.Fields(value)
		if seen[key] || (key != "Groups" && len(ids) != 4) {
			return false, errors.New("invalid process credentials")
		}
		seen[key] = true
		wanted := account.gid
		if key == "Uid" {
			wanted = account.uid
		}
		for _, id := range ids {
			if _, err := strconv.ParseUint(id, 10, 32); err != nil {
				return false, errors.New("invalid process credential ID")
			}
			busy = busy || (wanted != "" && id == wanted)
		}
	}
	if len(seen) != 3 {
		return false, errors.New("process credentials unavailable")
	}
	return busy, nil
}
