// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

// Progress reports a self-update's stages and download on one writer: a
// redrawn percentage bar on a terminal, occasional plain lines otherwise.
type Progress struct {
	writer      io.Writer
	interactive bool
	downloading bool
	lastDraw    time.Time
	lastBucket  int64
	lineWidth   int
}

func NewProgress(writer io.Writer) *Progress {
	return &Progress{writer: writer, interactive: Interactive(writer)}
}

// Report prints one stage or download step.
func (progress *Progress) Report(event selfupdate.Progress) {
	if event.Stage == selfupdate.StageDownload {
		progress.download(event)
		return
	}
	progress.Finish()
	var message string
	switch event.Stage {
	case selfupdate.StageCheckRelease:
		message = "Checking for updates..."
	case selfupdate.StageVerifyRelease:
		message = fmt.Sprintf("New release %s; downloading and verifying release signature...", event.Version)
	case selfupdate.StagePrepare:
		message = "Preparing update and waiting for exclusive access..."
	case selfupdate.StageVerifyBinary:
		message = "Verifying downloaded binary..."
	case selfupdate.StageInstall:
		message = "Installing update..."
	default:
		return
	}
	// Progress is best effort: a closed diagnostic stream must not interrupt an
	// atomic update or turn an installed release into a reported failure.
	_, _ = fmt.Fprintln(progress.writer, message)
}

func (progress *Progress) download(event selfupdate.Progress) {
	percent := 0
	bucket := event.Downloaded / (1 << 20)
	if event.Total > 0 {
		percent = min(99, int(100*event.Downloaded/event.Total))
		if event.Complete {
			percent = 100
		}
		bucket = int64(percent / 10)
	}
	now := time.Now()
	if progress.downloading && !event.Complete {
		if progress.interactive && now.Sub(progress.lastDraw) < 100*time.Millisecond {
			return
		}
		if !progress.interactive && bucket == progress.lastBucket {
			return
		}
	}
	message := fmt.Sprintf("Downloading %.1f MiB (total unknown)", float64(event.Downloaded)/(1<<20))
	if event.Total > 0 {
		message = fmt.Sprintf("Downloading %3d%%  %.1f / %.1f MiB", percent, float64(event.Downloaded)/(1<<20), float64(event.Total)/(1<<20))
		if progress.interactive {
			const width = 20
			filled := percent * width / 100
			message = fmt.Sprintf("Downloading [%s%s] %3d%%  %.1f / %.1f MiB",
				strings.Repeat("=", filled), strings.Repeat("-", width-filled), percent,
				float64(event.Downloaded)/(1<<20), float64(event.Total)/(1<<20))
		}
	}
	if progress.interactive {
		_, _ = fmt.Fprintf(progress.writer, "\r%s%s", message, strings.Repeat(" ", max(0, progress.lineWidth-len(message))))
		progress.lineWidth = len(message)
	} else {
		_, _ = fmt.Fprintln(progress.writer, message)
	}
	progress.downloading = true
	progress.lastDraw = now
	progress.lastBucket = bucket
	if event.Complete {
		progress.Finish()
	}
}

// Finish ends a redrawn download line; it is safe to call more than once.
func (progress *Progress) Finish() {
	if progress.downloading && progress.interactive {
		_, _ = fmt.Fprintln(progress.writer)
	}
	progress.downloading = false
	progress.lineWidth = 0
}

// UpdateError classifies a self-update failure.
func UpdateError(err error) error {
	switch {
	case err == nil, errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, fs.ErrPermission), errors.Is(err, selfupdate.ErrExecutableNotWritable):
		return Fail(ErrorPermission, "update_permission_denied", err)
	case errors.Is(err, selfupdate.ErrUnsupportedPlatform), errors.Is(err, selfupdate.ErrInvalidVersion):
		return Fail(ErrorUnavailable, "update_unsupported", err)
	case errors.Is(err, selfupdate.ErrReleaseUnavailable), errors.Is(err, selfupdate.ErrAssetMissing):
		return Fail(ErrorUnavailable, "update_release_unavailable", err)
	case errors.Is(err, selfupdate.ErrVerificationKeyInvalid), errors.Is(err, selfupdate.ErrReleaseInvalid),
		errors.Is(err, selfupdate.ErrSignatureInvalid), errors.Is(err, selfupdate.ErrChecksumInvalid),
		errors.Is(err, selfupdate.ErrExecutableInvalid), errors.Is(err, selfupdate.ErrExecutableChanged),
		errors.Is(err, selfupdate.ErrStagedExecutableInvalid), errors.Is(err, selfupdate.ErrStagedFileInvalid),
		errors.Is(err, selfupdate.ErrVersionNotNewer):
		return Fail(ErrorValidation, "update_validation_failed", err)
	default:
		return Fail(ErrorDomain, "update_failed", err)
	}
}
