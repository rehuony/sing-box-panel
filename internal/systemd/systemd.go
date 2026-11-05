// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemd implements the Linux systemd lifecycle of one service:
// the panel's or a node's. A Definition names the service, supplies its
// audited templates and the state that identifies an installation, and the
// Manager installs, controls, inspects and removes the unit, its dedicated
// system account and that account's ownership of the service's files.
package systemd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrUnsupportedOS = errors.New("systemd lifecycle is supported only on Linux")
	ErrPermission    = errors.New("system scope requires root privileges")
	ErrNotInstalled  = errors.New("systemd unit is not installed")
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

// Definition describes one service's systemd installation. Its state path
// identifies an installation: it is the value of StateFlag in the unit's
// ExecStart, and in system scope it is SystemStatePath.
type Definition[T any] struct {
	// Name names the unit (Name.service), the system account and its group,
	// the sysusers and tmpfiles declarations, the executable, the default
	// system data directory and the runtime directory.
	Name string
	// AccountDescription is the account comment in the sysusers template.
	AccountDescription string
	// SystemUnit, Sysusers and Tmpfiles are the audited system templates;
	// UserUnit is the user template, nil for a service without user scope.
	SystemUnit, UserUnit, Sysusers, Tmpfiles []byte
	// SystemStatePath is the state path of a system installation.
	SystemStatePath string
	// StateFlag and StateShortFlag name the ExecStart flag that carries the
	// state path; StateShortFlag may be empty.
	StateFlag, StateShortFlag string
	// StateLabel names the state path in messages, such as "settings".
	StateLabel string
	// StateFile reports that the state path is a file, whose directory the
	// service also writes; otherwise the state path is the data directory.
	StateFile bool
	// Arguments renders the ExecStart arguments after the executable around
	// the quoted state path.
	Arguments func(quotedStatePath string) string
	State     State[T]
}

// State is what a service keeps outside its unit and the lifecycle consults:
// how a state path determines the data directory, which state the service
// creates and owns, and how a pending move of its data proceeds.
type State[T any] interface {
	// DataDir returns the data directory of the installation at statePath
	// without creating anything. fallback is the scope's default data
	// directory, empty for user scope; now requires state the service can
	// start with.
	DataDir(statePath, fallback string, now bool) (string, error)
	// Validate checks service-specific constraints on resolved paths before
	// anything is written.
	Validate(executable, statePath, dataDir string) error
	// Location reports the data directory the installation uses now and the
	// one its state selects; move reports a relocation in progress. An
	// empty current means the state records none.
	Location(statePath string) (current, target string, move bool, err error)
	// Relocate moves the data to dataDir before the unit is written, when
	// the state selects another directory.
	Relocate(ctx context.Context, statePath, dataDir string) error
	// Prepare creates missing state and returns what it created, nil when it
	// created nothing.
	Prepare(ctx context.Context, statePath, dataDir string, now bool) (*T, error)
	// Owned lists the state paths besides the data directory that the system
	// account owns.
	Owned(statePath string) []OwnedPath
	// Retained lists directories besides the data directories whose
	// ownership returns to root when the account is removed.
	Retained(statePath string) ([]string, error)
	// Lock excludes the service's state writers while ownership moves; it
	// returns nil when the service has none.
	Lock(ctx context.Context, statePath string) (io.Closer, error)
}

// OwnedPath is a state path the system account owns.
type OwnedPath struct {
	Path string
	Mode os.FileMode
	// Directory is a directory rather than a regular file.
	Directory bool
	// Optional paths may be missing.
	Optional bool
}

type Layout struct {
	SystemUnitPath       string
	SystemSysusersPath   string
	SystemTmpfilesPath   string
	SystemExecutablePath string
	SystemStatePath      string
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

// Service is the lifecycle a command line drives.
type Service[T any] interface {
	Files(context.Context, Scope) (FilesResult, error)
	Install(context.Context, InstallRequest) (InstallResult[T], error)
	Uninstall(context.Context, UninstallRequest) (UninstallResult, error)
	Status(context.Context, Scope) (Status, error)
	Control(context.Context, Scope, Action) (ControlResult[T], error)
	Logs(context.Context, LogsRequest) (LogsResult, error)
}

type FileStatus struct {
	Path     string `json:"path"`
	State    string `json:"state"`
	Managed  bool   `json:"managed"`
	Retained bool   `json:"retained,omitempty"`
}

type FilesResult struct {
	Scope     Scope        `json:"scope"`
	StatePath string       `json:"state_path,omitempty"`
	Files     []FileStatus `json:"files"`
}

type Options struct {
	GOOS       string
	EUID       func() int
	Executable func() (string, error)
	Runner     Runner
	Layout     Layout
	LookPath   func(string) (string, error)
	// ProcRoot is where process credentials are read; empty means /proc.
	ProcRoot string
}

type Manager[T any] struct {
	definition Definition[T]
	unitName   string
	goos       string
	euid       func() int
	executable func() (string, error)
	runner     Runner
	layout     Layout
	lookPath   func(string) (string, error)
	procRoot   string
}

func New[T any](definition Definition[T], options Options) (*Manager[T], error) {
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
		options.Runner = ExecRunner{}
	}
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.ProcRoot == "" {
		options.ProcRoot = "/proc"
	}
	layout, err := resolvedLayout(definition, options.Layout)
	if err != nil {
		return nil, err
	}
	return &Manager[T]{
		definition: definition, unitName: definition.Name + ".service",
		goos: options.GOOS, euid: options.EUID, executable: options.Executable,
		runner: options.Runner, layout: layout, lookPath: options.LookPath, procRoot: options.ProcRoot,
	}, nil
}

// Unit is the name of the service's unit.
func (manager *Manager[T]) Unit() string { return manager.unitName }

type InstallRequest struct {
	Scope     Scope  `json:"scope"`
	StatePath string `json:"state_path"`
	DataDir   string `json:"data_dir"`
	Force     bool   `json:"force"`
	Now       bool   `json:"now"`
}

type InstallResult[T any] struct {
	// Initial is the state Prepare created, if any.
	Initial         *T       `json:"-"`
	StateCreated    bool     `json:"state_created"`
	Scope           Scope    `json:"scope"`
	Unit            string   `json:"unit"`
	UnitPath        string   `json:"unit_path"`
	ExecutablePath  string   `json:"executable_path"`
	StatePath       string   `json:"state_path"`
	DataDir         string   `json:"data_dir"`
	InstalledPaths  []string `json:"installed_paths"`
	Enabled         bool     `json:"enabled"`
	Started         bool     `json:"started"`
	PersistentState bool     `json:"persistent_state_preserved"`
}

type UninstallRequest struct {
	Scope    Scope `json:"scope"`
	Force    bool  `json:"force"`
	KeepUser bool  `json:"keep_user"`
}

type UninstallResult struct {
	Scope            Scope    `json:"scope"`
	Unit             string   `json:"unit"`
	RemovedPaths     []string `json:"removed_paths"`
	Stopped          bool     `json:"stopped"`
	Disabled         bool     `json:"disabled"`
	ConfigRetained   bool     `json:"config_retained"`
	DataRetained     bool     `json:"data_retained"`
	AccountInspected bool     `json:"account_inspected"`
	AccountRetained  bool     `json:"account_retained"`
	GroupRetained    bool     `json:"group_retained"`
	AccountRemoved   bool     `json:"account_removed"`
	GroupRemoved     bool     `json:"group_removed"`
	AccountNote      string   `json:"account_note,omitempty"`
}

// Status is systemd's own view of the unit plus one fact read from disk.
// UnitFileStatePath is the state path argument written in the unit file at
// UnitPath as it exists now; it says nothing about the command line systemd
// loaded (see NeedDaemonReload) or the state the running process started
// with. It is empty when the file does not state one unambiguous path.
type Status struct {
	Scope             Scope  `json:"scope"`
	Unit              string `json:"unit"`
	UnitPath          string `json:"unit_path"`
	LoadState         string `json:"load_state"`
	ActiveState       string `json:"active_state"`
	SubState          string `json:"sub_state"`
	UnitFileState     string `json:"unit_file_state"`
	MainPID           int    `json:"main_pid"`
	NeedDaemonReload  bool   `json:"need_daemon_reload"`
	UnitFileStatePath string `json:"unit_file_state_path,omitempty"`
}

type ControlResult[T any] struct {
	// Initial is the state recreated before a start, if any.
	Initial *T     `json:"-"`
	Scope   Scope  `json:"scope"`
	Unit    string `json:"unit"`
	Action  Action `json:"action"`
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

// managedMark is the persisted ownership marker of every installed file.
// Keep it stable across renames so existing installations stay recognized.
func (manager *Manager[T]) managedMark() string {
	return "# Managed by " + manager.definition.Name + " system install."
}
