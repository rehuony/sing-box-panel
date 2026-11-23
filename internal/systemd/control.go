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
	"time"
)

// ValidateStatePath checks the on-disk state identity without treating stale
// or customized unit definitions as evidence of the effective state. label
// names the state path, such as "settings".
func (status Status) ValidateStatePath(expected, label string) error {
	command := "systemctl"
	if status.Scope == ScopeUser {
		command += " --user"
	}
	inspect := command + " cat " + status.Unit
	if status.NeedDaemonReload {
		return fmt.Errorf("%w: systemd reports NeedDaemonReload=yes\nInspect: %s\nNext: after reviewing the unit, run `%s daemon-reload` and retry", ErrConflict, inspect, command)
	}
	if status.LoadState == "not-found" {
		return nil
	}
	if status.UnitFileStatePath == "" {
		return fmt.Errorf("%w: service %s path is unknown (check ExecStart and drop-ins)\nInspect: %s", ErrConflict, label, inspect)
	}
	if filepath.Clean(status.UnitFileStatePath) != filepath.Clean(expected) {
		return fmt.Errorf("%w: service uses different %s\nExpected: %s\nUnit file: %s\nInspect: %s", ErrConflict, label, expected, status.UnitFileStatePath, inspect)
	}
	return nil
}

// Status reports the service's unit and, for a system installation, its
// companion units.
func (manager *Manager[T]) Status(ctx context.Context, requested Scope) (Status, error) {
	status, err := manager.unitStatus(ctx, requested)
	if err != nil || status.Scope != ScopeSystem {
		return status, err
	}
	for _, companion := range manager.definition.Companions {
		state, err := manager.companionStatus(ctx, companion.Name)
		if err != nil {
			return status, err
		}
		status.Companions = append(status.Companions, state)
	}
	return status, nil
}

func (manager *Manager[T]) unitStatus(ctx context.Context, requested Scope) (Status, error) {
	status, err := manager.queryStatus(ctx, requested)
	if err == nil && status.LoadState == "not-found" {
		return status, fmt.Errorf("%w: %s", ErrNotInstalled, manager.unitName)
	}
	return status, err
}

func (manager *Manager[T]) companionStatus(ctx context.Context, name string) (UnitState, error) {
	args := append(manager.systemctlPrefix(ScopeSystem), "show", name, "--no-pager",
		"--property=LoadState", "--property=ActiveState", "--property=SubState", "--property=UnitFileState")
	result, err := manager.runResult(ctx, "systemctl", args...)
	if err != nil {
		return UnitState{}, err
	}
	state := UnitState{Unit: name}
	for _, line := range strings.Split(strings.TrimSpace(string(result.Stdout)), "\n") {
		key, value, _ := strings.Cut(line, "=")
		switch key {
		case "LoadState":
			state.LoadState = value
		case "ActiveState":
			state.ActiveState = value
		case "SubState":
			state.SubState = value
		case "UnitFileState":
			state.UnitFileState = value
		}
	}
	if state.LoadState == "" || state.ActiveState == "" {
		return UnitState{}, fmt.Errorf("%w: systemctl omitted the state of %s", ErrInvalid, name)
	}
	return state, nil
}

// RequestRestart queues a restart of the service without waiting for it,
// so the service itself can ask for one: systemd stops it as part of the
// job.
func (manager *Manager[T]) RequestRestart(ctx context.Context, scope Scope) error {
	if err := manager.requireLinux(); err != nil {
		return err
	}
	scope, err := manager.resolveScope(scope)
	if err != nil {
		return err
	}
	return manager.runSystemctl(ctx, scope, "--no-block", "restart", manager.unitName)
}

// ErrNotActive reports a service that did not stay active after a restart.
var ErrNotActive = errors.New("the service did not stay active")

// restartPoll is how often RestartAndWait reads the service's state.
var restartPoll = time.Second

// RestartAndWait restarts the service and waits until systemd reports it
// active throughout settle. timeout bounds becoming active: the service
// fails with ErrNotActive when it is not active once timeout has passed, or
// when it stops being active before settle ends; a settle period that began
// in time runs to its end.
func (manager *Manager[T]) RestartAndWait(ctx context.Context, scope Scope, timeout, settle time.Duration) error {
	if err := manager.requireLinux(); err != nil {
		return err
	}
	scope, err := manager.resolveScope(scope)
	if err != nil {
		return err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	if err := manager.runSystemctl(ctx, scope, "restart", manager.unitName); err != nil {
		return fmt.Errorf("%w: %w", ErrNotActive, err)
	}
	var activeSince time.Time
	for {
		status, err := manager.queryStatus(ctx, scope)
		if err != nil {
			return err
		}
		now := time.Now()
		state := status.ActiveState + "/" + status.SubState
		switch {
		case status.ActiveState == "active" && activeSince.IsZero():
			activeSince = now
		case status.ActiveState == "active":
		case !activeSince.IsZero():
			return fmt.Errorf("%w: %s became %s within %s of starting", ErrNotActive, manager.unitName, state, settle)
		case now.After(deadline):
			return fmt.Errorf("%w: %s is %s after %s", ErrNotActive, manager.unitName, state, timeout)
		}
		if !activeSince.IsZero() && now.Sub(activeSince) >= settle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(restartPoll):
		}
	}
}

func (manager *Manager[T]) queryStatus(ctx context.Context, requested Scope) (Status, error) {
	if err := manager.requireLinux(); err != nil {
		return Status{}, err
	}
	scope, err := manager.resolveScope(requested)
	if err != nil {
		return Status{}, err
	}
	args := manager.systemctlPrefix(scope)
	args = append(args,
		"show", manager.unitName, "--no-pager",
		"--property=LoadState", "--property=ActiveState", "--property=SubState",
		"--property=UnitFileState", "--property=MainPID", "--property=FragmentPath",
		"--property=DropInPaths", "--property=NeedDaemonReload",
	)
	result, err := manager.runResult(ctx, "systemctl", args...)
	if err != nil {
		return Status{}, err
	}
	properties, err := parseProperties(result.Stdout)
	if err != nil {
		return Status{}, err
	}
	pid, err := strconv.Atoi(properties["MainPID"])
	if err != nil || pid < 0 {
		return Status{}, fmt.Errorf("%w: systemctl returned invalid MainPID %q", ErrInvalid, properties["MainPID"])
	}
	unitPath := properties["FragmentPath"]
	if unitPath != "" || properties["LoadState"] == "loaded" {
		if _, err := cleanAbsolute(unitPath, "systemctl FragmentPath"); err != nil {
			return Status{}, err
		}
	}
	status := Status{
		Scope: scope, Unit: manager.unitName, UnitPath: unitPath,
		LoadState: properties["LoadState"], ActiveState: properties["ActiveState"],
		SubState: properties["SubState"], UnitFileState: properties["UnitFileState"], MainPID: pid,
		NeedDaemonReload: properties["NeedDaemonReload"] == "yes",
	}
	// Drop-ins may override ExecStart, so the fragment alone does not state
	// the unit's command line; leave the path unknown in that case.
	if inactiveDropIns(properties["DropInPaths"]) {
		status.UnitFileStatePath, _ = manager.unitFileStatePath(unitPath)
	}
	return status, nil
}

func (manager *Manager[T]) Control(ctx context.Context, requested Scope, action Action) (result ControlResult[T], controlErr error) {
	if err := manager.requireLinux(); err != nil {
		return ControlResult[T]{}, err
	}
	scope, err := manager.resolveScope(requested)
	if err != nil {
		return ControlResult[T]{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return ControlResult[T]{}, err
	}
	switch action {
	case ActionStart, ActionStop, ActionRestart:
	default:
		return ControlResult[T]{}, fmt.Errorf("%w: unsupported control action %q", ErrInvalid, action)
	}
	if err := manager.prepareDataMove(ctx, scope, action); err != nil {
		return ControlResult[T]{}, err
	}
	if action != ActionStop {
		initial, err := manager.prepareInstalledResources(ctx, scope)
		defer func() { result.Initial = initial }()
		if err != nil {
			return ControlResult[T]{}, err
		}
	}
	if err := manager.runSystemctl(ctx, scope, string(action), manager.unitName); err != nil {
		return ControlResult[T]{}, err
	}
	return ControlResult[T]{Scope: scope, Unit: manager.unitName, Action: action}, nil
}

func (manager *Manager[T]) Logs(ctx context.Context, request LogsRequest) (LogsResult, error) {
	if err := manager.requireLinux(); err != nil {
		return LogsResult{}, err
	}
	scope, err := manager.resolveScope(request.Scope)
	if err != nil {
		return LogsResult{}, err
	}
	if request.Lines < 1 || request.Lines > 100_000 {
		return LogsResult{}, fmt.Errorf("%w: lines must be between 1 and 100000", ErrInvalid)
	}
	if hasControl(request.Since) {
		return LogsResult{}, fmt.Errorf("%w: since must not contain control characters", ErrInvalid)
	}
	args := []string{"--no-pager", "--output=short-iso", "--lines=" + strconv.Itoa(request.Lines)}
	if scope == ScopeUser {
		args = append(args, "--user-unit="+manager.unitName)
	} else {
		args = append(args, "--unit="+manager.unitName)
	}
	if strings.TrimSpace(request.Since) != "" {
		args = append(args, "--since", request.Since)
	}
	result, err := manager.runResult(ctx, "journalctl", args...)
	if err != nil {
		return LogsResult{}, err
	}
	return LogsResult{
		Scope: scope, Unit: manager.unitName, Lines: request.Lines, Since: request.Since,
		Text: strings.TrimSuffix(string(result.Stdout), "\n"),
	}, nil
}

// Empty/masked drop-ins have no directives and cannot override ExecStart.
// Unknown quoting, unreadable paths and any nonempty customization stay ambiguous.
func inactiveDropIns(value string) bool {
	for _, path := range strings.Fields(value) {
		if strings.ContainsAny(path, `\"`) {
			return false
		}
		if target, err := os.Readlink(path); err == nil && target == "/dev/null" {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
			return false
		}
	}
	return true
}
