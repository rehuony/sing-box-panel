// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	systemdassets "github.com/rehuony/sing-box-panel/systemd"
)

// The installer reserves this non-login identity. A matching name alone is
// insufficient: never adopt or delete a login account, root, or a shared ID.
type systemAccount struct {
	uid string
	gid string
}

func (manager *Manager) inspectSystemAccount(ctx context.Context) (systemAccount, error) {
	passwd, err := manager.runResult(ctx, "getent", "passwd")
	if err != nil {
		return systemAccount{}, err
	}
	groups, err := manager.runResult(ctx, "getent", "group")
	if err != nil {
		return systemAccount{}, err
	}
	var account systemAccount
	var primaryGroup string
	var userRecord, groupRecord string
	users := strings.FieldsFunc(string(passwd.Stdout), func(r rune) bool { return r == '\n' })
	groupLines := strings.FieldsFunc(string(groups.Stdout), func(r rune) bool { return r == '\n' })
	for _, line := range users {
		fields := strings.Split(line, ":")
		if fields[0] != serviceUser {
			continue
		}
		if len(fields) != 7 || account.uid != "" || !validServiceID(fields[2]) ||
			fields[4] != "Sing-Box Panel service account" || fields[5] != "/var/lib/sing-box-panel" || fields[6] != "/usr/sbin/nologin" {
			return account, accountConflict("user does not match the dedicated sysusers profile")
		}
		account.uid, primaryGroup = fields[2], fields[3]
		userRecord = line
	}
	for _, line := range groupLines {
		fields := strings.Split(line, ":")
		if fields[0] != serviceGroup {
			continue
		}
		if len(fields) != 4 || account.gid != "" || !validServiceID(fields[2]) {
			return account, accountConflict("invalid dedicated group")
		}
		for _, member := range strings.Split(fields[3], ",") {
			if member != "" && member != serviceUser {
				return account, accountConflict("service group has other members")
			}
		}
		account.gid = fields[2]
		groupRecord = line
	}
	if account.uid != "" && (account.gid == "" || primaryGroup != account.gid) {
		return account, accountConflict("user does not have the dedicated primary group")
	}
	for _, line := range users {
		fields := strings.Split(line, ":")
		if len(fields) != 7 {
			return account, accountConflict("cannot inspect passwd entries")
		}
		if fields[0] != serviceUser && ((account.uid != "" && fields[2] == account.uid) || (account.gid != "" && fields[3] == account.gid)) {
			return account, accountConflict("service UID or GID is shared by another user")
		}
	}
	for _, line := range groupLines {
		fields := strings.Split(line, ":")
		if len(fields) != 4 {
			return account, accountConflict("cannot inspect group entries")
		}
		if fields[0] != serviceGroup && account.gid != "" && fields[2] == account.gid {
			return account, accountConflict("service GID is shared by another group")
		}
	}
	// NSS enumeration can omit entries that resolve by name. Confirm the exact
	// names before adopting an identity or claiming that deletion succeeded.
	for _, entry := range []struct{ database, name, record string }{
		{"passwd", serviceUser, userRecord}, {"group", serviceGroup, groupRecord},
	} {
		result, err := manager.runResult(ctx, "getent", entry.database, entry.name)
		var status interface{ ExitCode() int }
		if entry.record == "" && errors.As(err, &status) && status.ExitCode() == 2 {
			continue // getent: the supplied key was not found.
		}
		if err != nil {
			return account, err
		}
		if strings.TrimSpace(string(result.Stdout)) != entry.record {
			return account, accountConflict("account lookup disagrees with account enumeration")
		}
	}
	return account, nil
}

func validServiceID(value string) bool {
	id, err := strconv.ParseUint(value, 10, 32)
	return err == nil && id > 0 && id < 1<<32-1 && strconv.FormatUint(id, 10) == value
}

func accountConflict(reason string) error {
	return fmt.Errorf("%w: %s; inspect the service account or uninstall with --keep-user", ErrConflict, reason)
}

// Presence does not imply installer ownership. Retention also works for a
// customized account, but must not claim a nonexistent identity was retained.
func (manager *Manager) inspectAccountPresence(ctx context.Context, result *UninstallResult) error {
	for _, entry := range []struct {
		database string
		fields   int
		present  *bool
	}{
		{"passwd", 7, &result.AccountRetained},
		{"group", 4, &result.GroupRetained},
	} {
		lookup, err := manager.runResult(ctx, "getent", entry.database, serviceUser)
		var status interface{ ExitCode() int }
		if errors.As(err, &status) && status.ExitCode() == 2 {
			continue
		}
		if err != nil {
			return err
		}
		record := strings.TrimSpace(string(lookup.Stdout))
		if record == "" {
			continue
		}
		fields := strings.Split(record, ":")
		if len(fields) != entry.fields || fields[0] != serviceUser || strings.Contains(record, "\n") {
			return accountConflict("cannot determine service account presence")
		}
		*entry.present = true
	}
	result.AccountInspected = true
	return nil
}

func (manager *Manager) preflightAccountRemoval(ctx context.Context, keepUser bool, result *UninstallResult) (systemAccount, error) {
	// Retain unverified identities. --force only authorizes replacing/removing
	// service files; it must never authorize deleting an unrelated OS account.
	if err := manager.inspectAccountPresence(ctx, result); err != nil {
		return systemAccount{}, err
	}
	if keepUser {
		if result.AccountRetained || result.GroupRetained {
			result.AccountNote = "account and ownership retained by --keep-user"
		}
		return systemAccount{}, nil
	}
	info, err := os.Lstat(manager.layout.SystemSysusersPath)
	if errors.Is(err, os.ErrNotExist) {
		if result.AccountRetained || result.GroupRetained {
			result.AccountNote = "no installer-owned sysusers declaration; account not modified"
		}
		return systemAccount{}, nil
	}
	if err != nil {
		return systemAccount{}, err
	}
	if !info.Mode().IsRegular() {
		return systemAccount{}, accountConflict("sysusers declaration is not a regular file")
	}
	declaration, err := os.ReadFile(manager.layout.SystemSysusersPath)
	if err != nil {
		return systemAccount{}, err
	}
	if !bytes.Equal(declaration, systemdassets.Sysusers) {
		return systemAccount{}, accountConflict("sysusers declaration is customized")
	}
	account, err := manager.inspectSystemAccount(ctx)
	if err != nil {
		return account, err
	}
	result.AccountRetained, result.GroupRetained = account.uid != "", account.gid != ""
	var commands []string
	if account.uid != "" {
		commands = append(commands, "userdel")
	}
	if account.gid != "" {
		commands = append(commands, "groupdel")
	}
	if account.uid != "" || account.gid != "" {
		commands = append(commands, "chown")
	}
	for _, name := range commands {
		if _, err := manager.lookPath(name); err != nil {
			return account, fmt.Errorf("required command %s is unavailable: %w", name, err)
		}
	}
	return account, nil
}

func (manager *Manager) validateAccountRemovalStatus(status Status) error {
	if status.NeedDaemonReload || (status.LoadState != "not-found" &&
		(status.LoadState != "loaded" || status.UnitPath != manager.layout.SystemUnitPath || status.UnitFileSettingsPath != manager.layout.SystemSettingsPath)) {
		return accountConflict("service settings are ambiguous or changed on disk; resolve the effective service configuration before account removal")
	}
	return nil
}

// Called only after systemd confirms termination and disable succeeds. Account
// cleanup precedes service-file deletion so a failure remains retryable.
func (manager *Manager) removeSystemAccount(ctx context.Context, expected systemAccount, result *UninstallResult) error {
	if expected.uid == "" && expected.gid == "" {
		return nil
	}
	current, err := manager.inspectSystemAccount(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return accountConflict("service account changed during uninstall")
	}
	status, err := manager.queryStatus(ctx, ScopeSystem)
	if err != nil {
		return err
	}
	if err := manager.validateAccountRemovalStatus(status); err != nil {
		return err
	}
	if !unitStopped(status) {
		return accountConflict("service started again during uninstall")
	}
	// Serialize configuration writers and exclude database owners (including
	// manually launched root processes) before transferring retained ownership.
	if fileExists(filepath.Dir(manager.layout.SystemSettingsPath)) {
		lock, err := settings.TryLock(ctx, manager.layout.SystemSettingsPath)
		if err != nil {
			return err
		}
		defer lock.Close()
	}
	paths, err := manager.retainedAccountPaths()
	if err != nil {
		return fmt.Errorf("cannot protect retained data before account removal (use --keep-user to retain ownership): %w", err)
	}
	for _, path := range paths {
		lock, err := store.LockDirectoryForCleanup(path)
		if err != nil {
			return err
		}
		defer lock.Close()
	}
	if err := manager.requireAccountIdle(ctx, current); err != nil {
		return err
	}
	for _, path := range paths {
		// Numeric IDs avoid name-resolution changes. -P and --no-dereference
		// never follow certificate/file symlinks outside the retained roots.
		if current.uid != "" {
			if err := manager.run(ctx, "chown", "--recursive", "--no-dereference", "-P", "--from=+"+current.uid, "+0", "--", path); err != nil {
				return err
			}
		}
		if current.gid != "" {
			if err := manager.run(ctx, "chown", "--recursive", "--no-dereference", "-P", "--from=:+"+current.gid, ":+0", "--", path); err != nil {
				return err
			}
		}
	}
	if current.uid != "" {
		// No --force or --remove. The independent process check above also
		// covers hosts where userdel cannot inspect another process's root.
		deleteErr := manager.run(ctx, "userdel", "--", serviceUser)
		after, inspectErr := manager.inspectSystemAccount(ctx)
		result.AccountInspected = inspectErr == nil
		result.AccountRetained, result.GroupRetained = false, false
		if inspectErr == nil {
			result.AccountRetained, result.GroupRetained = after.uid != "", after.gid != ""
			result.AccountRemoved = after.uid == ""
			result.GroupRemoved = current.gid != "" && after.gid == ""
		}
		if err := errors.Join(deleteErr, inspectErr); err != nil {
			return err
		}
		if after.uid != "" || (after.gid != "" && after.gid != current.gid) {
			return accountConflict("service account was not removed or changed during removal")
		}
		current = after
	}
	// userdel may already remove the empty private group (USERGROUPS_ENAB).
	if current.gid != "" {
		deleteErr := manager.run(ctx, "groupdel", "--", serviceGroup)
		after, inspectErr := manager.inspectSystemAccount(ctx)
		result.AccountInspected = inspectErr == nil
		result.AccountRetained, result.GroupRetained = false, false
		if inspectErr == nil {
			result.AccountRetained, result.GroupRetained = after.uid != "", after.gid != ""
			result.GroupRemoved = after.gid == ""
		}
		if err := errors.Join(deleteErr, inspectErr); err != nil {
			return err
		}
		if after.uid != "" || after.gid != "" {
			return accountConflict("service account reappeared during removal")
		}
	}
	return nil
}

func (manager *Manager) retainedAccountPaths() ([]string, error) {
	settingsPath := manager.layout.SystemSettingsPath
	if err := preflightSettingsPaths(settingsPath); err != nil {
		return nil, err
	}
	paths := []string{filepath.Dir(settingsPath), manager.layout.SystemDataDir}
	configured, err := settings.ConfiguredDataDir(settingsPath)
	if err == nil {
		paths = append(paths, configured)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	location, err := settings.ReadDataLocation(settingsPath)
	if err == nil {
		paths = append(paths, location.DataDir)
		if location.Move != nil {
			paths = append(paths, location.Move.Target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	unit, err := os.ReadFile(manager.layout.SystemUnitPath)
	if err == nil {
		if path, known := parseUnitFileSettingsPath(unit); !known || path != settingsPath {
			return nil, accountConflict("installed service settings are unknown or different")
		}
		for _, line := range strings.Split(string(unit), "\n") {
			if value, ok := strings.CutPrefix(line, "WorkingDirectory="); ok {
				if strings.Contains(strings.ReplaceAll(value, "%%", ""), "%") {
					return nil, accountConflict("unresolved working-directory specifier")
				}
				paths = append(paths, strings.ReplaceAll(value, "%%", "%"))
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	retained := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := cleanAbsolute(path, "retained directory"); err != nil {
			return nil, err
		}
		if err := installation.ValidateCleanup(installation.Report{
			SettingsPath: settingsPath, DataDir: path,
			Entries: []installation.Entry{{Path: manager.layout.SystemExecutablePath}},
		}); err != nil {
			return nil, err
		}
		if err := requireDirectory(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		retained = append(retained, path)
	}
	return retained, nil
}
