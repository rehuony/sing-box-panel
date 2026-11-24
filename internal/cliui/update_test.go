// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

func TestProgressInteractiveBarAndCleanup(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := &Progress{writer: &output, interactive: true}
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Total: 2 << 20})
	if !strings.Contains(output.String(), "\rDownloading [--------------------]   0%") {
		t.Fatal(output.String())
	}
	// Intermediate draws are throttled; completion must always be flushed.
	progress.lastDraw = time.Now().Add(time.Hour)
	before := output.String()
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20, Total: 2 << 20})
	if output.String() != before {
		t.Fatal("intermediate progress was not throttled")
	}
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 2 << 20, Total: 2 << 20, Complete: true})
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageVerifyBinary})
	if !strings.Contains(output.String(), "[====================] 100%  2.0 / 2.0 MiB\nVerifying") {
		t.Fatal(output.String())
	}
	output.Reset()
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Total: 2 << 20})
	progress.Finish() // Same cleanup used before the command returns an error.
	progress.Finish()
	if !strings.HasSuffix(output.String(), "MiB\n") || strings.Count(output.String(), "\n") != 1 || strings.Contains(output.String(), "100%") {
		t.Fatalf("failed download cleanup = %q", output.String())
	}
}

func TestProgressUnknownSizeAndRedirectedThrottling(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := NewProgress(&output)
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload})
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 19})
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20})
	if strings.Contains(output.String(), "%") || !strings.Contains(output.String(), "1.0 MiB (total unknown)") || strings.Count(output.String(), "\n") != 2 {
		t.Fatal(output.String())
	}
	progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20, Total: 1 << 20, Complete: true})
	if !strings.Contains(output.String(), "100%") || strings.ContainsAny(output.String(), "\r\x1b") {
		t.Fatal(output.String())
	}
	output.Reset()
	for percent := range 100 {
		progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: int64(percent), Total: 100})
	}
	if got := strings.Count(output.String(), "\n"); got != 10 {
		t.Fatalf("redirected download printed %d lines, want 10", got)
	}
}

func TestProgressDoesNotCompleteBeforeEOF(t *testing.T) {
	t.Parallel()
	for _, total := range []int64{10, 5} {
		var output bytes.Buffer
		progress := NewProgress(&output)
		progress.Report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 10, Total: total})
		if strings.Contains(output.String(), "100%") || !strings.Contains(output.String(), "99%") {
			t.Fatal(output.String())
		}
	}
}
