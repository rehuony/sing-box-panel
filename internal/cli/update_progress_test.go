// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/selfupdate"
)

func TestUpdateProgressInteractiveBarAndCleanup(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := &updateProgress{writer: &output, interactive: true}
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Total: 2 << 20})
	if !strings.Contains(output.String(), "\rDownloading [--------------------]   0%") {
		t.Fatal(output.String())
	}
	// Intermediate draws are throttled; completion must always be flushed.
	progress.lastDraw = time.Now().Add(time.Hour)
	before := output.String()
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20, Total: 2 << 20})
	if output.String() != before {
		t.Fatal("intermediate progress was not throttled")
	}
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 2 << 20, Total: 2 << 20, Complete: true})
	progress.report(selfupdate.Progress{Stage: selfupdate.StageVerifyBinary})
	if !strings.Contains(output.String(), "[====================] 100%  2.0 / 2.0 MiB\nVerifying") {
		t.Fatal(output.String())
	}
	output.Reset()
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Total: 2 << 20})
	progress.finish() // Same cleanup used before the command returns an error.
	progress.finish()
	if !strings.HasSuffix(output.String(), "MiB\n") || strings.Count(output.String(), "\n") != 1 || strings.Contains(output.String(), "100%") {
		t.Fatalf("failed download cleanup = %q", output.String())
	}
}

func TestUpdateProgressUnknownSizeAndRedirectedThrottling(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := newUpdateProgress(&output)
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload})
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 19})
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20})
	if strings.Contains(output.String(), "%") || !strings.Contains(output.String(), "1.0 MiB (total unknown)") || strings.Count(output.String(), "\n") != 2 {
		t.Fatal(output.String())
	}
	progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 1 << 20, Total: 1 << 20, Complete: true})
	if !strings.Contains(output.String(), "100%") || strings.ContainsAny(output.String(), "\r\x1b") {
		t.Fatal(output.String())
	}
	output.Reset()
	for percent := range 100 {
		progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: int64(percent), Total: 100})
	}
	if got := strings.Count(output.String(), "\n"); got != 10 {
		t.Fatalf("redirected download printed %d lines, want 10", got)
	}
}

func TestUpdateProgressDoesNotCompleteBeforeEOF(t *testing.T) {
	t.Parallel()
	for _, total := range []int64{10, 5} {
		var output bytes.Buffer
		progress := newUpdateProgress(&output)
		progress.report(selfupdate.Progress{Stage: selfupdate.StageDownload, Downloaded: 10, Total: total})
		if strings.Contains(output.String(), "100%") || !strings.Contains(output.String(), "99%") {
			t.Fatal(output.String())
		}
	}
}
