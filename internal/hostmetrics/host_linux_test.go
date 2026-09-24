// SPDX-License-Identifier: GPL-3.0-or-later
//go:build linux

package hostmetrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSamplerUsesDedicatedHostCounters(t *testing.T) {
	proc := t.TempDir()
	t.Setenv("SING_BOX_PANEL_HOST_PROC", proc)
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(proc, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("stat", "cpu 100 0 0 400\n")
	write("meminfo", "MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	write("loadavg", "0.25 0.10 0.05 1/20 42\n")
	var sampler Sampler
	first := sampler.Sample(t.TempDir())
	if first.CPUPercent != nil || first.MemoryTotal == nil || *first.MemoryTotal != 1024000 ||
		first.MemoryUsed == nil || *first.MemoryUsed != 614400 || first.LoadOne == nil || *first.LoadOne != 0.25 {
		t.Fatalf("first sample = %+v", first)
	}
	write("stat", "cpu 120 0 0 480\n")
	// Expire the cache without waiting on wall-clock time.
	sampler.cached.SampledAt = time.Now().Add(-2 * time.Second)
	next := sampler.Sample(t.TempDir())
	if next.CPUPercent == nil || *next.CPUPercent != 20 {
		t.Fatalf("CPU utilization = %v, want 20%%", next.CPUPercent)
	}
}

func TestMissingHostCountersDoNotFallBackToDifferentEvidence(t *testing.T) {
	t.Setenv("SING_BOX_PANEL_HOST_PROC", t.TempDir())
	snapshot, cpu := readHost(t.TempDir())
	if cpu != nil || snapshot.LoadOne != nil || snapshot.MemoryTotal != nil || snapshot.MemoryUsed != nil {
		t.Fatalf("missing host counters produced evidence: %+v, %+v", snapshot, cpu)
	}
	if snapshot.DiskTotal == nil || snapshot.DiskUsed == nil {
		t.Fatal("unavailable procfs counters must not prevent disk sampling")
	}
}

// Run the compiled test binary under the packaged system unit to verify the
// real mount namespace. Ordinary unit tests use fixtures and skip this probe.
func TestSystemServiceHostCounters(t *testing.T) {
	if os.Getenv("SING_BOX_PANEL_HOST_PROC") != "/run/sing-box-panel/host-proc" {
		t.Skip("requires the packaged system service mount namespace")
	}
	if _, err := os.Stat("/proc/stat"); !os.IsNotExist(err) {
		t.Fatalf("ProcSubset=pid is not effective: %v", err)
	}
	for _, name := range []string{"stat", "meminfo", "loadavg"} {
		var mount unix.Statfs_t
		if err := unix.Statfs(filepath.Join(os.Getenv("SING_BOX_PANEL_HOST_PROC"), name), &mount); err != nil || mount.Flags&unix.ST_RDONLY == 0 {
			t.Fatalf("host counter %s is not mounted read-only: %v", name, err)
		}
	}
	var sampler Sampler
	first := sampler.Sample(".")
	if first.MemoryTotal == nil || first.MemoryUsed == nil || first.LoadOne == nil || first.DiskTotal == nil || first.DiskUsed == nil {
		t.Fatalf("host counters are unavailable inside the system service: %+v", first)
	}
	time.Sleep(1100 * time.Millisecond)
	second := sampler.Sample(".")
	if second.CPUPercent == nil || *second.CPUPercent < 0 || *second.CPUPercent > 100 {
		t.Fatalf("CPU utilization is unavailable: %+v", second)
	}
}
