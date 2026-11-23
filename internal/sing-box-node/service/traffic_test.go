// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

func TestTrafficAccumulatesAcrossCoreRestartsWithoutGuessing(t *testing.T) {
	start := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
	reading := func(pid int, token string, seconds int, upload, download int64) coreTraffic {
		return coreTraffic{
			identity:  RuntimeIdentity{PID: pid, ProcessStartToken: token, ActivationBundleID: "bundle_a"},
			sampledAt: start.Add(time.Duration(seconds) * time.Second), upload: upload, download: download,
		}
	}
	steps := []struct {
		name             string
		reading          coreTraffic
		upload, download int64
		decreased        bool
	}{
		// The first reading of a process is its baseline.
		{"baseline", reading(10, "a", 0, 500, 900), 0, 0, false},
		{"growth", reading(10, "a", 1, 700, 1000), 200, 100, false},
		// A restarted core's first reading is its baseline: traffic before it is not guessed.
		{"restart", reading(11, "b", 2, 50, 60), 200, 100, false},
		{"after restart", reading(11, "b", 3, 80, 100), 230, 140, false},
		// A decrease within one process rebases it without lowering the totals.
		{"decrease", reading(11, "b", 4, 10, 20), 230, 140, true},
		{"after decrease", reading(11, "b", 5, 15, 30), 235, 150, false},
	}
	var checkpoint store.TrafficCheckpoint
	sampled := false
	for _, step := range steps {
		var decreased bool
		checkpoint, decreased = advanceTraffic(checkpoint, sampled, step.reading)
		sampled = true
		if checkpoint.UploadTotal != step.upload || checkpoint.DownloadTotal != step.download || decreased != step.decreased {
			t.Fatalf("%s: totals = %d/%d decreased=%v, want %d/%d decreased=%v", step.name,
				checkpoint.UploadTotal, checkpoint.DownloadTotal, decreased, step.upload, step.download, step.decreased)
		}
	}
}
