// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemd implements the Linux systemd lifecycle used by the CLI.
// It never invokes a shell: every external command and argument crosses the
// injected Runner boundary separately.
package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	systemdassets "github.com/rehuony/sing-box-panel/packaging/systemd"
)

const (
	UnitName     = "sing-box-panel.service"
	managedMark  = "# Managed by sing-box-panel system install."
	serviceUser  = "sing-box-panel"
	serviceGroup = "sing-box-panel"
)

var (
	ErrUnsupportedOS = errors.New("systemd lifecycle is supported only on Linux")
	ErrPermission    = errors.New("system scope requires root privileges")
	ErrNotInstalled  = errors.New("sing-box-panel systemd unit is not installed")
	ErrConflict      = errors.New("managed systemd destination conflicts with existing content")
	ErrInvalid       = errors.New("invalid systemd request")
)

type Scope string

const (
	ScopeAuto   Scope = "auto"
	ScopeSystem Scope = "system"
	ScopeUser   Scope = "user"
)

func ParseScope(value string) (Scope, error) {
	scope := Scope(strings.ToLower(strings.TrimSpace(value)))
	switch scope {
	case ScopeAuto, ScopeSystem, ScopeUser:
		return scope, nil
	default:
		return "", fmt.Errorf("%w: scope must be auto, system, or user", ErrInvalid)
	}
}

type Action string

const (
	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionRestart Action = "restart"
)

type Layout struct {
	SystemUnitPath       string
	SystemSysusersPath   string
	SystemTmpfilesPath   string
	SystemExecutablePath string
	SystemSettingsPath   string
	SystemDataDir        string
	UserUnitPath         string
}

type CommandResult struct {
	Stdout []byte
	Stderr []byte
}

type Runner interface {
	Run(context.Context, string, ...string) (CommandResult, error)
}

type Service interface {
	Install(context.Context, InstallRequest) (InstallResult, error)
	Uninstall(context.Context, UninstallRequest) (UninstallResult, error)
	Status(context.Context, Scope) (Status, error)
	Control(context.Context, Scope, Action) (ControlResult, error)
	Logs(context.Context, LogsRequest) (LogsResult, error)
}

type Options struct {
	GOOS       string
	EUID       func() int
	Executable func() (string, error)
	Runner     Runner
	Layout     Layout
}

type Manager struct {
	goos       string
	euid       func() int
	executable func() (string, error)
	runner     Runner
	layout     Layout
}

func New(options Options) (*Manager, error) {
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.EUID == nil {
		options.EUID = os.Geteuid
	}
	if options.Executable == nil {
		options.Executable = os.Executable
	}
	if options.Runner == nil {
		options.Runner = execRunner{}
	}
	layout, err := resolvedLayout(options.Layout)
	if err != nil {
		return nil, err
	}
	return &Manager{
		goos: options.GOOS, euid: options.EUID, executable: options.Executable,
		runner: options.Runner, layout: layout,
	}, nil
}

type InstallRequest struct {
	Scope        Scope  `json:"scope"`
	SettingsPath string `json:"settings_path"`
	DataDir      string `json:"data_dir"`
	Force        bool   `json:"force"`
	Now          bool   `json:"now"`
}

type InstallResult struct {
	Scope           Scope    `json:"scope"`
	Unit            string   `json:"unit"`
	UnitPath        string   `json:"unit_path"`
	ExecutablePath  string   `json:"executable_path"`
	SettingsPath    string   `json:"settings_path"`
	DataDir         string   `json:"data_dir"`
	InstalledPaths  []string `json:"installed_paths"`
	Enabled         bool     `json:"enabled"`
	Started         bool     `json:"started"`
	PersistentState bool     `json:"persistent_state_preserved"`
}

type UninstallRequest struct {
	Scope Scope `json:"scope"`
	Force bool  `json:"force"`
}

type UninstallResult struct {
	Scope           Scope    `json:"scope"`
	Unit            string   `json:"unit"`
	RemovedPaths    []string `json:"removed_paths"`
	Stopped         bool     `json:"stopped"`
	Disabled        bool     `json:"disabled"`
	ConfigRetained  bool     `json:"config_retained"`
	DataRetained    bool     `json:"data_retained"`
	AccountRetained bool     `json:"account_retained"`
}

type Status struct {
	Scope         Scope  `json:"scope"`
	Unit          string `json:"unit"`
	UnitPath      string `json:"unit_path"`
	LoadState     string `json:"load_state"`
	ActiveState   string `json:"active_state"`
	SubState      string `json:"sub_state"`
	UnitFileState string `json:"unit_file_state"`
	MainPID       int    `json:"main_pid"`
}

type ControlResult struct {
	Scope  Scope  `json:"scope"`
	Unit   string `json:"unit"`
	Action Action `json:"action"`
}

type LogsRequest struct {
	Scope Scope  `json:"scope"`
	Lines int    `json:"lines"`
	Since string `json:"since,omitempty"`
}

type LogsResult struct {
	Scope Scope  `json:"scope"`
	Unit  string `json:"unit"`
	Lines int    `json:"lines"`
	Since string `json:"since,omitempty"`
	Text  string `json:"text"`
}

type CommandError struct {
	Name   string
	Args   []string
	Stderr string
	Cause  error
}

func (err *CommandError) Error() string {
	message := fmt.Sprintf("%s %s failed", err.Name, strings.Join(err.Args, " "))
	if err.Stderr != "" {
		message += ": " + err.Stderr
	}
	return message
}

func (err *CommandError) Unwrap() error { return err.Cause }

func (manager *Manager) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
	if err := manager.requireLinux(); err != nil {
		return InstallResult{}, err
	}
	scope, err := manager.resolveScope(request.Scope)
	if err != nil {
		return InstallResult{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return InstallResult{}, err
	}
	executablePath, settingsPath, dataDir, err := manager.validateInstallPaths(scope, request)
	if err != nil {
		return InstallResult{}, err
	}

	unit, err := renderUnit(scope, executablePath, settingsPath, dataDir)
	if err != nil {
		return InstallResult{}, err
	}
	files := manager.installFiles(scope, unit)
	if err := preflightInstall(files, request.Force); err != nil {
		return InstallResult{}, err
	}
	installed := make([]string, 0, len(files))
	for _, file := range files {
		changed, writeErr := installFile(file.path, file.data)
		if writeErr != nil {
			return InstallResult{}, writeErr
		}
		if changed {
			installed = append(installed, file.path)
		}
	}

	if scope == ScopeSystem {
		if err := manager.run(ctx, "systemd-sysusers", manager.layout.SystemSysusersPath); err != nil {
			return InstallResult{}, err
		}
		if err := manager.run(ctx, "systemd-tmpfiles", "--create", manager.layout.SystemTmpfilesPath); err != nil {
			return InstallResult{}, err
		}
		if err := manager.run(ctx, "chown", "root:"+serviceGroup, settingsPath); err != nil {
			return InstallResult{}, err
		}
		if err := os.Chmod(settingsPath, 0o640); err != nil {
			return InstallResult{}, fmt.Errorf("set system settings permissions %q: %w", settingsPath, err)
		}
		if err := manager.run(ctx, "chown", "--recursive", "--no-dereference", serviceUser+":"+serviceGroup, dataDir); err != nil {
			return InstallResult{}, err
		}
	}
	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return InstallResult{}, err
	}
	enableArgs := []string{"enable"}
	if request.Now {
		enableArgs = append(enableArgs, "--now")
	}
	enableArgs = append(enableArgs, UnitName)
	if err := manager.runSystemctl(ctx, scope, enableArgs...); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		Scope: scope, Unit: UnitName, UnitPath: manager.unitPath(scope), ExecutablePath: executablePath,
		SettingsPath: settingsPath, DataDir: dataDir, InstalledPaths: installed,
		Enabled: true, Started: request.Now, PersistentState: true,
	}, nil
}

func (manager *Manager) Uninstall(ctx context.Context, request UninstallRequest) (UninstallResult, error) {
	if err := manager.requireLinux(); err != nil {
		return UninstallResult{}, err
	}
	scope, err := manager.resolveScope(request.Scope)
	if err != nil {
		return UninstallResult{}, err
	}
	if err := manager.requireMutationPermission(scope); err != nil {
		return UninstallResult{}, err
	}
	paths := manager.managedPaths(scope)
	existing, err := preflightUninstall(paths, request.Force)
	if err != nil {
		return UninstallResult{}, err
	}
	if len(existing) == 0 {
		return UninstallResult{}, ErrNotInstalled
	}
	unitPresent := fileExists(manager.unitPath(scope))
	if unitPresent {
		if err := manager.runSystemctl(ctx, scope, "disable", "--now", UnitName); err != nil {
			return UninstallResult{}, err
		}
	}
	removed := make([]string, 0, len(existing))
	for _, path := range existing {
		if err := os.Remove(path); err != nil {
			return UninstallResult{}, fmt.Errorf("remove managed systemd file %q: %w", path, err)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return UninstallResult{}, err
		}
		removed = append(removed, path)
	}
	if err := manager.runSystemctl(ctx, scope, "daemon-reload"); err != nil {
		return UninstallResult{}, err
	}
	return UninstallResult{
		Scope: scope, Unit: UnitName, RemovedPaths: removed, Stopped: unitPresent, Disabled: unitPresent,
		ConfigRetained: true, DataRetained: true, AccountRetained: scope == ScopeSystem,
	}, nil
}

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

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

func (manager *Manager) requireLinux() error {
	if manager.goos != "linux" {
		return fmt.Errorf("%w: running on %s", ErrUnsupportedOS, manager.goos)
	}
	return nil
}

func (manager *Manager) resolveScope(scope Scope) (Scope, error) {
	parsed, err := ParseScope(string(scope))
	if err != nil {
		return "", err
	}
	if parsed == ScopeAuto {
		if manager.euid() == 0 {
			return ScopeSystem, nil
		}
		return ScopeUser, nil
	}
	return parsed, nil
}

func (manager *Manager) requireMutationPermission(scope Scope) error {
	if scope == ScopeSystem && manager.euid() != 0 {
		return ErrPermission
	}
	return nil
}

func (manager *Manager) validateInstallPaths(scope Scope, request InstallRequest) (string, string, string, error) {
	executablePath, err := manager.executable()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve current executable: %w", err)
	}
	executablePath, err = cleanAbsolute(executablePath, "executable path")
	if err != nil {
		return "", "", "", err
	}
	settingsPath, err := cleanAbsolute(request.SettingsPath, "settings path")
	if err != nil {
		return "", "", "", err
	}
	dataDir, err := cleanAbsolute(request.DataDir, "data directory")
	if err != nil {
		return "", "", "", err
	}
	if scope == ScopeSystem {
		if executablePath != manager.layout.SystemExecutablePath || settingsPath != manager.layout.SystemSettingsPath || dataDir != manager.layout.SystemDataDir {
			return "", "", "", fmt.Errorf(
				"%w: system scope requires executable=%q settings=%q data=%q",
				ErrInvalid, manager.layout.SystemExecutablePath, manager.layout.SystemSettingsPath, manager.layout.SystemDataDir,
			)
		}
	}
	if err := requireRegularExecutable(executablePath); err != nil {
		return "", "", "", err
	}
	if err := requireRegularFile(settingsPath, "settings"); err != nil {
		return "", "", "", err
	}
	if err := requireDirectory(dataDir); err != nil {
		return "", "", "", err
	}
	return executablePath, settingsPath, dataDir, nil
}

func (manager *Manager) installFiles(scope Scope, unit []byte) []managedFile {
	files := []managedFile{{path: manager.unitPath(scope), data: unit}}
	if scope == ScopeSystem {
		files = append(files,
			managedFile{path: manager.layout.SystemSysusersPath, data: systemdassets.Sysusers},
			managedFile{path: manager.layout.SystemTmpfilesPath, data: systemdassets.Tmpfiles},
		)
	}
	return files
}

func (manager *Manager) managedPaths(scope Scope) []string {
	paths := []string{manager.unitPath(scope)}
	if scope == ScopeSystem {
		paths = append(paths, manager.layout.SystemSysusersPath, manager.layout.SystemTmpfilesPath)
	}
	return paths
}

func (manager *Manager) unitPath(scope Scope) string {
	if scope == ScopeSystem {
		return manager.layout.SystemUnitPath
	}
	return manager.layout.UserUnitPath
}

func (manager *Manager) systemctlPrefix(scope Scope) []string {
	args := []string{"--no-ask-password"}
	if scope == ScopeUser {
		args = append(args, "--user")
	}
	return args
}

func (manager *Manager) runSystemctl(ctx context.Context, scope Scope, args ...string) error {
	all := append(manager.systemctlPrefix(scope), args...)
	return manager.run(ctx, "systemctl", all...)
}

func (manager *Manager) run(ctx context.Context, name string, args ...string) error {
	_, err := manager.runResult(ctx, name, args...)
	return err
}

func (manager *Manager) runResult(ctx context.Context, name string, args ...string) (CommandResult, error) {
	result, err := manager.runner.Run(ctx, name, args...)
	if err != nil {
		return CommandResult{}, &CommandError{
			Name: name, Args: append([]string(nil), args...),
			Stderr: strings.TrimSpace(string(result.Stderr)), Cause: err,
		}
	}
	return result, nil
}

func resolvedLayout(layout Layout) (Layout, error) {
	configDir, err := os.UserConfigDir()
	if err != nil && layout.UserUnitPath == "" {
		return Layout{}, fmt.Errorf("resolve user config directory: %w", err)
	}
	defaults := Layout{
		SystemUnitPath:       "/etc/systemd/system/" + UnitName,
		SystemSysusersPath:   "/etc/sysusers.d/sing-box-panel.conf",
		SystemTmpfilesPath:   "/etc/tmpfiles.d/sing-box-panel.conf",
		SystemExecutablePath: "/usr/local/bin/sing-box-panel",
		SystemSettingsPath:   "/etc/sing-box-panel/setting.json",
		SystemDataDir:        "/var/lib/sing-box-panel",
		UserUnitPath:         filepath.Join(configDir, "systemd", "user", UnitName),
	}
	if layout.SystemUnitPath != "" {
		defaults.SystemUnitPath = layout.SystemUnitPath
	}
	if layout.SystemSysusersPath != "" {
		defaults.SystemSysusersPath = layout.SystemSysusersPath
	}
	if layout.SystemTmpfilesPath != "" {
		defaults.SystemTmpfilesPath = layout.SystemTmpfilesPath
	}
	if layout.SystemExecutablePath != "" {
		defaults.SystemExecutablePath = layout.SystemExecutablePath
	}
	if layout.SystemSettingsPath != "" {
		defaults.SystemSettingsPath = layout.SystemSettingsPath
	}
	if layout.SystemDataDir != "" {
		defaults.SystemDataDir = layout.SystemDataDir
	}
	if layout.UserUnitPath != "" {
		defaults.UserUnitPath = layout.UserUnitPath
	}
	for name, value := range map[string]string{
		"system unit path": defaults.SystemUnitPath, "system sysusers path": defaults.SystemSysusersPath,
		"system tmpfiles path": defaults.SystemTmpfilesPath, "system executable path": defaults.SystemExecutablePath,
		"system settings path": defaults.SystemSettingsPath, "system data directory": defaults.SystemDataDir,
		"user unit path": defaults.UserUnitPath,
	} {
		if _, err := cleanAbsolute(value, name); err != nil {
			return Layout{}, err
		}
	}
	return defaults, nil
}

func cleanAbsolute(value, name string) (string, error) {
	if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) || hasControl(value) {
		return "", fmt.Errorf("%w: %s must be an absolute path without control characters", ErrInvalid, name)
	}
	clean := filepath.Clean(value)
	if clean != value {
		return "", fmt.Errorf("%w: %s must be normalized", ErrInvalid, name)
	}
	return clean, nil
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func requireRegularExecutable(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect executable %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%w: executable path %q must be a regular executable file", ErrInvalid, path)
	}
	return nil
}

func requireRegularFile(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s %q must be a regular file", ErrInvalid, label, path)
	}
	return nil
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect data directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: data directory %q must be a directory", ErrInvalid, path)
	}
	return nil
}

func renderUnit(scope Scope, executablePath, settingsPath, dataDir string) ([]byte, error) {
	template := systemdassets.SystemUnit
	if scope == ScopeUser {
		template = systemdassets.UserUnit
	}
	executable, err := quoteExecArgument(executablePath)
	if err != nil {
		return nil, err
	}
	settings, err := quoteExecArgument(settingsPath)
	if err != nil {
		return nil, err
	}
	data, err := quotePathDirective(dataDir)
	if err != nil {
		return nil, err
	}
	result, err := replaceDirective(template, "ExecStart=", "ExecStart="+executable+" server run --config "+settings)
	if err != nil {
		return nil, err
	}
	result, err = replaceDirective(result, "WorkingDirectory=", "WorkingDirectory="+data)
	if err != nil {
		return nil, err
	}
	if scope == ScopeUser {
		result, err = replaceDirective(result, "ReadWritePaths=", "ReadWritePaths="+data)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func quoteExecArgument(value string) (string, error) {
	return quoteUnitValue(value, true)
}

func quotePathDirective(value string) (string, error) {
	return quoteUnitValue(value, false)
}

func quoteUnitValue(value string, escapeDollar bool) (string, error) {
	if value == "" || hasControl(value) {
		return "", fmt.Errorf("%w: systemd arguments must not be empty or contain control characters", ErrInvalid)
	}
	var output strings.Builder
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\\', '"':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '%':
			output.WriteString("%%")
		case '$':
			if escapeDollar {
				output.WriteString("$$")
			} else {
				output.WriteRune(character)
			}
		default:
			output.WriteRune(character)
		}
	}
	output.WriteByte('"')
	return output.String(), nil
}

func replaceDirective(source []byte, prefix, replacement string) ([]byte, error) {
	lines := strings.Split(string(source), "\n")
	replaced := 0
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[index] = replacement
			replaced++
		}
	}
	if replaced != 1 {
		return nil, fmt.Errorf("%w: template has %d %s directives", ErrInvalid, replaced, prefix)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

type managedFile struct {
	path string
	data []byte
}

func preflightInstall(files []managedFile, force bool) error {
	for _, file := range files {
		info, err := os.Lstat(file.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect managed destination %q: %w", file.path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: destination %q is not a regular file", ErrConflict, file.path)
		}
		existing, err := os.ReadFile(file.path)
		if err != nil {
			return fmt.Errorf("read managed destination %q: %w", file.path, err)
		}
		if !bytes.Equal(existing, file.data) && !force {
			return fmt.Errorf("%w: %q; pass --force to replace it", ErrConflict, file.path)
		}
	}
	return nil
}

func preflightUninstall(paths []string, force bool) ([]string, error) {
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect managed destination %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: destination %q is not a regular file", ErrConflict, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read managed destination %q: %w", path, err)
		}
		if !bytes.Contains(data, []byte(managedMark)) && !force {
			return nil, fmt.Errorf("%w: refusing to remove unmanaged file %q; pass --force to remove it", ErrConflict, path)
		}
		existing = append(existing, path)
	}
	return existing, nil
}

func installFile(path string, data []byte) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("%w: destination %q is not a regular file", ErrConflict, path)
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("read managed destination %q: %w", path, err)
		}
		if bytes.Equal(existing, data) {
			if info.Mode().Perm() == 0o644 {
				return false, nil
			}
			if err := os.Chmod(path, 0o644); err != nil {
				return false, fmt.Errorf("set managed-file permissions %q: %w", path, err)
			}
			if err := syncDirectory(filepath.Dir(path)); err != nil {
				return false, err
			}
			return true, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect managed destination %q: %w", path, err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, fmt.Errorf("create systemd directory %q: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".sing-box-panel-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create temporary managed file in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return false, fmt.Errorf("set temporary managed-file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return false, fmt.Errorf("write temporary managed file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return false, fmt.Errorf("sync temporary managed file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close temporary managed file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, fmt.Errorf("replace managed file %q: %w", path, err)
	}
	if err := syncDirectory(directory); err != nil {
		return false, err
	}
	return true, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open managed directory %q: %w", path, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync managed directory %q: %w", path, err)
	}
	return nil
}

func parseProperties(data []byte) (map[string]string, error) {
	properties := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("%w: malformed systemctl property %q", ErrInvalid, line)
		}
		properties[key] = value
	}
	for _, required := range []string{"LoadState", "ActiveState", "SubState", "UnitFileState", "MainPID", "FragmentPath"} {
		if _, found := properties[required]; !found {
			return nil, fmt.Errorf("%w: systemctl omitted %s", ErrInvalid, required)
		}
	}
	return properties, nil
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
