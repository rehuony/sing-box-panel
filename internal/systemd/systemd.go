// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemd implements the Linux systemd lifecycle used by the CLI.
package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
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
