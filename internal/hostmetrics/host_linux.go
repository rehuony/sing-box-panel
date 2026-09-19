// SPDX-License-Identifier: GPL-3.0-or-later
//go:build linux

package hostmetrics

import (
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func readHost(path string) (*Snapshot, *cpuTimes) {
	result := &Snapshot{CPUCount: runtime.NumCPU()}
	cpu, _ := os.ReadFile("/proc/stat")
	memory, _ := os.ReadFile("/proc/meminfo")
	result.MemoryTotal, result.MemoryUsed = parseMemory(string(memory))
	load, err := os.ReadFile("/proc/loadavg")
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
