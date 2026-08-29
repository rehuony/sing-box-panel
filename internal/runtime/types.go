// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtime owns validation, materialization, lifecycle, and restricted
// monitoring access for the single sing-box child process. It does not select,
// project, or persist an activation bundle.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrRuntime               = errors.New("runtime operation failed")
	ErrUnavailable           = errors.New("runtime process execution is unavailable")
	ErrInvalidBundle         = errors.New("invalid applied bundle")
	ErrArtifactDigest        = errors.New("artifact digest verification failed")
	ErrStartupConfigDigest   = errors.New("startup config digest verification failed")
	ErrVersionMismatch       = errors.New("sing-box exact version mismatch")
	ErrCheckFailed           = errors.New("sing-box config check failed")
	ErrHealthFailed          = errors.New("sing-box health check failed")
	ErrProcessExited         = errors.New("sing-box process exited")
	ErrNotRunning            = errors.New("sing-box process is not running")
	ErrAlreadyRunning        = errors.New("sing-box process is already active")
	ErrClosed                = errors.New("runtime manager is closed")
	ErrNotClosed             = errors.New("runtime manager must be closed before waiting")
	ErrCommandOutputTooLarge = errors.New("runtime command output exceeds its limit")
	ErrMaterialization       = errors.New("startup config materialization failed")
	ErrTermination           = errors.New("sing-box process termination failed")
)

// AppliedBundle contains only immutable inputs selected by the application
// layer. Start and Restart never derive or replace any of these values.
type AppliedBundle struct {
	ID                  string
	ArtifactID          string
	ExactVersion        coreartifact.ExactVersion
	ArtifactDigest      coreartifact.SHA256
	BinaryPath          string
	StartupConfig       []byte
	StartupConfigDigest coreartifact.SHA256
}

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateFailed   State = "failed"
)

type MonitoringLevel string

const (
	MonitoringLimited     MonitoringLevel = "limited"
	MonitoringProcessOnly MonitoringLevel = "process_only"
)

// HealthStatus reports only evidence supplied by the configured probe. It has
// no metrics fields, so process-only operation cannot fabricate measurements.
type HealthStatus struct {
	Level     MonitoringLevel
	Healthy   bool
	Code      string
	CheckedAt time.Time
}

type FailureStatus struct {
	Operation string
	Code      string
	FailedAt  time.Time
}

// Snapshot is an immutable copy of current lifecycle state. Requested and
// actual identities are separate so a failed verification remains explicit.
type Snapshot struct {
	State                   State
	BundleID                string
	RequestedArtifactID     string
	ActualArtifactID        string
	RequestedExactVersion   coreartifact.ExactVersion
	ActualExactVersion      coreartifact.ExactVersion
	RequestedArtifactDigest coreartifact.SHA256
	ActualArtifactDigest    coreartifact.SHA256
	StartupConfigDigest     coreartifact.SHA256
	PID                     int
	StartedAt               time.Time
	TransitionedAt          time.Time
	Health                  *HealthStatus
	Failure                 *FailureStatus
}

// LiveIdentity is the read-only process identity observed by this manager.
// Persistence and independent OS-process identity reconciliation belong to
// the application layer; this value never substitutes desired configuration.
type LiveIdentity struct {
	Running        bool
	State          State
	PID            int
	ExactVersion   coreartifact.ExactVersion
	ArtifactDigest coreartifact.SHA256
	ArtifactID     string
	BundleID       string
	StartedAt      time.Time
}

// Failure keeps the public error text free of command output and filesystem
// paths while retaining error identity through Unwrap.
type Failure struct {
	Operation string
	Code      string
	cause     error
}

func (failure *Failure) Error() string {
	return fmt.Sprintf("runtime %s failed (%s)", failure.Operation, failure.Code)
}

func (failure *Failure) Unwrap() error { return failure.cause }

func fail(operation, code string, kind, cause error) error {
	return &Failure{
		Operation: operation,
		Code:      code,
		cause:     errors.Join(ErrRuntime, kind, cause),
	}
}

type Command struct {
	Path   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// ChildProcess represents one process group. Wait must be called exactly once
// by its owner; Terminate and Kill target the complete group on Linux.
type ChildProcess interface {
	PID() int
	Wait() error
	Terminate() error
	Kill() error
}

type CommandExecutor interface {
	Run(context.Context, Command, int64) ([]byte, error)
	Start(Command) (ChildProcess, error)
}

type ProcessInfo struct {
	PID       int
	StartedAt time.Time
	Exited    <-chan struct{}
}

type HealthObservation struct {
	Healthy bool
	Code    string
}

// HealthProbe must return when ctx is canceled or process.Exited closes.
type HealthProbe interface {
	Level() MonitoringLevel
	AwaitHealthy(context.Context, ProcessInfo) (HealthObservation, error)
}

type Options struct {
	RuntimeDir           string
	Executor             CommandExecutor
	Clock                Clock
	Probe                HealthProbe
	Stdout               io.Writer
	Stderr               io.Writer
	ShutdownGrace        time.Duration
	ProcessHealthWindow  time.Duration
	MaximumBinaryBytes   int64
	MaximumConfigBytes   int64
	MaximumCommandOutput int64
}
