// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

type updateProgress struct {
	writer      io.Writer
	interactive bool
	downloading bool
	lastDraw    time.Time
	lastBucket  int64
	lineWidth   int
}

func newUpdateProgress(writer io.Writer) *updateProgress {
	progress := &updateProgress{writer: writer}
	if terminal, ok := writer.(interface{ Fd() uintptr }); ok && os.Getenv("TERM") != "dumb" {
		progress.interactive = isatty.IsTerminal(terminal.Fd())
	}
	return progress
}

func (progress *updateProgress) report(event selfupdate.Progress) {
	if event.Stage == selfupdate.StageDownload {
		progress.download(event)
		return
	}
	progress.finish()
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

func (progress *updateProgress) download(event selfupdate.Progress) {
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
		progress.finish()
	}
}

func (progress *updateProgress) finish() {
	if progress.downloading && progress.interactive {
		_, _ = fmt.Fprintln(progress.writer)
	}
	progress.downloading = false
	progress.lineWidth = 0
}
