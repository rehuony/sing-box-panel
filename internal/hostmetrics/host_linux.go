// SPDX-License-Identifier: GPL-3.0-or-later
//go:build linux

package hostmetrics

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func readHost(path string) (*Snapshot, *cpuTimes) {
	// The system service keeps ProcSubset=pid and bind-mounts only these
	// counters outside /proc. Direct and user-service runs use the normal procfs.
	procRoot := os.Getenv("SING_BOX_PANEL_HOST_PROC")
	if procRoot == "" {
		procRoot = "/proc"
	}
	result := &Snapshot{CPUCount: runtime.NumCPU()}
	cpu, _ := os.ReadFile(filepath.Join(procRoot, "stat"))
	memory, _ := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	result.MemoryTotal, result.MemoryUsed = parseMemory(string(memory))
	load, err := os.ReadFile(filepath.Join(procRoot, "loadavg"))
	if fields := strings.Fields(string(load)); err == nil && len(fields) > 0 {
		if n, err := strconv.ParseFloat(fields[0], 64); err == nil {
			result.LoadOne = &n
		}
	}
	if path == "" {
		path = "."
	}
	var disk unix.Statfs_t
	if unix.Statfs(path, &disk) == nil && disk.Bsize > 0 && disk.Blocks > 0 && disk.Bfree <= disk.Blocks && disk.Blocks <= ^uint64(0)/uint64(disk.Bsize) {
		total := disk.Blocks * uint64(disk.Bsize)
		used := (disk.Blocks - disk.Bfree) * uint64(disk.Bsize)
		result.DiskTotal = &total
		result.DiskUsed = &used
	}
	return result, parseCPU(string(cpu))
}
