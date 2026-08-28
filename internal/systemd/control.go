// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (manager *Manager) Status(ctx context.Context, requested Scope) (Status, error) {
	if err := manager.requireLinux(); err != nil {
		return Status{}, err
	}
	scope, err := manager.resolveScope(requested)
	if err != nil {
		return Status{}, err
	}
	args := manager.systemctlPrefix(scope)
	args = append(args,
		"show", UnitName, "--no-pager",
		"--property=LoadState", "--property=ActiveState", "--property=SubState",
		"--property=UnitFileState", "--property=MainPID", "--property=FragmentPath",
	)
	result, err := manager.runResult(ctx, "systemctl", args...)
	if err != nil {
		return Status{}, err
	}
	properties, err := parseProperties(result.Stdout)
	if err != nil {
		return Status{}, err
	}
	if properties["LoadState"] == "not-found" {
		return Status{}, ErrNotInstalled
	}
	pid, err := strconv.Atoi(properties["MainPID"])
	if err != nil || pid < 0 {
		return Status{}, fmt.Errorf("%w: systemctl returned invalid MainPID %q", ErrInvalid, properties["MainPID"])
	}
	unitPath := properties["FragmentPath"]
	if _, err := cleanAbsolute(unitPath, "systemctl FragmentPath"); err != nil {
		return Status{}, err
	}
	return Status{
		Scope: scope, Unit: UnitName, UnitPath: unitPath,
		LoadState: properties["LoadState"], ActiveState: properties["ActiveState"],
		SubState: properties["SubState"], UnitFileState: properties["UnitFileState"], MainPID: pid,
	}, nil
}

func (manager *Manager) Control(ctx context.Context, requested Scope, action Action) (ControlResult, error) {
	if err := manager.requireLinux(); err != nil {
		return ControlResult{}, err
	}
	scope, err := manager.resolveScope(requested)
	if err != nil {
		return ControlResult{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return ControlResult{}, err
	}
	switch action {
	case ActionStart, ActionStop, ActionRestart:
	default:
		return ControlResult{}, fmt.Errorf("%w: unsupported control action %q", ErrInvalid, action)
	}
	if err := manager.runSystemctl(ctx, scope, string(action), UnitName); err != nil {
		return ControlResult{}, err
	}
	return ControlResult{Scope: scope, Unit: UnitName, Action: action}, nil
}

func (manager *Manager) Logs(ctx context.Context, request LogsRequest) (LogsResult, error) {
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
		args = append(args, "--user-unit="+UnitName)
	} else {
		args = append(args, "--unit="+UnitName)
	}
	if strings.TrimSpace(request.Since) != "" {
		args = append(args, "--since", request.Since)
	}
	result, err := manager.runResult(ctx, "journalctl", args...)
	if err != nil {
		return LogsResult{}, err
	}
	return LogsResult{
		Scope: scope, Unit: UnitName, Lines: request.Lines, Since: request.Since,
		Text: strings.TrimSuffix(string(result.Stdout), "\n"),
	}, nil
}
