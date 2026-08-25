// SPDX-License-Identifier: GPL-3.0-or-later

// Package artifactstore safely verifies and publishes immutable sing-box
// binaries into a content-addressed local store.
package artifactstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrArtifactStore   = errors.New("artifact store operation failed")
	ErrUnsafeSource    = errors.New("local artifact source is unsafe")
	ErrUnsafeExecution = errors.New("artifact version execution cannot be safely bounded")
	ErrUnsafeURL       = errors.New("artifact download URL is unsafe")
	ErrTooLarge        = errors.New("artifact exceeds a resource limit")
	ErrDigest          = errors.New("artifact digest verification failed")
	ErrArchive         = errors.New("artifact archive verification failed")
	ErrELF             = errors.New("artifact ELF verification failed")
	ErrVersion         = errors.New("artifact version verification failed")
	ErrCorruptStore    = errors.New("content-addressed artifact store is corrupt")
)

type Step string

const (
	StepPrepare  Step = "prepare"
	StepDownload Step = "download"
	StepDigest   Step = "digest"
	StepArchive  Step = "archive"
	StepELF      Step = "elf"
	StepVersion  Step = "version"
	StepPublish  Step = "publish"
)

// Failure intentionally omits the underlying error text. This keeps signed
// URLs, local paths, subprocess output, and credentials out of routine logs.
type Failure struct {
	Step  Step
	Code  string
	Cause error
}

func (failure *Failure) Error() string {
	return fmt.Sprintf("artifact %s failed (%s)", failure.Step, failure.Code)
}

func (failure *Failure) Unwrap() error { return failure.Cause }

func fail(step Step, code string, cause error) error {
	return &Failure{Step: step, Code: code, Cause: errors.Join(ErrArtifactStore, cause)}
}

type Diagnostic struct {
	Step    Step   `json:"step"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Result struct {
	Identity           coreartifact.Identity
	BinarySHA256       coreartifact.SHA256
	BinaryPath         string
	ArchivePath        string
	FeatureFingerprint FeatureFingerprint
	Diagnostics        []Diagnostic
}

type Limits struct {
	MaximumArchiveBytes  int64
	MaximumExpandedBytes int64
	MaximumFileBytes     int64
	MaximumFiles         int
	DownloadTimeout      time.Duration
	VersionTimeout       time.Duration
	MaximumVersionOutput int64
}

func DefaultLimits() Limits {
	return Limits{
		MaximumArchiveBytes:  128 << 20,
		MaximumExpandedBytes: 256 << 20,
		MaximumFileBytes:     128 << 20,
		MaximumFiles:         128,
		DownloadTimeout:      2 * time.Minute,
		VersionTimeout:       10 * time.Second,
		MaximumVersionOutput: 64 << 10,
	}
}

func (limits Limits) validate() error {
	if limits.MaximumArchiveBytes <= 0 || limits.MaximumArchiveBytes > 2<<30 ||
		limits.MaximumExpandedBytes <= 0 || limits.MaximumExpandedBytes > 4<<30 ||
		limits.MaximumFileBytes <= 0 || limits.MaximumFileBytes > limits.MaximumExpandedBytes ||
		limits.MaximumFiles <= 0 || limits.MaximumFiles > 10_000 ||
		limits.DownloadTimeout <= 0 || limits.DownloadTimeout > 10*time.Minute ||
		limits.VersionTimeout <= 0 || limits.VersionTimeout > time.Minute ||
		limits.MaximumVersionOutput <= 0 || limits.MaximumVersionOutput > 1<<20 {
		return fmt.Errorf("invalid artifact resource limits")
	}
	return nil
}

type Downloader interface {
	Download(ctx context.Context, rawURL string, destination io.Writer, maximumBytes int64) (int64, error)
}

type VersionReport struct {
	Version            coreartifact.ExactVersion
	FeatureFingerprint FeatureFingerprint
}

type VersionInspector interface {
	// Inspect executes or otherwise interrogates the exact candidate copy. An
	// implementation is a trusted security boundary; use an OS-sandboxed
	// implementation when candidates are not fully administrator-trusted.
	Inspect(ctx context.Context, binaryPath string, maximumOutput int64) (VersionReport, error)
}

type Options struct {
	// Root's parent must already exist and have trusted ownership/mode. New
	// creates only the final Root directory and returns its canonical path from
	// Store.Root.
	Root       string
	Downloader Downloader
	Inspector  VersionInspector
	Limits     Limits
}

// ImportRequest describes an administrator-trusted local archive. ImportLocal
// runs an isolated copy's `version` command; SHA-256 pins bytes but does not
// make arbitrary code safe.
type ImportRequest struct {
	SourcePath           string
	SourceDescription    string
	ExpectedSHA256       coreartifact.SHA256
	ExpectedVersion      coreartifact.ExactVersion
	ExpectedArchitecture coreartifact.Architecture
	Variant              coreartifact.Variant
}
