// SPDX-License-Identifier: GPL-3.0-or-later

// Package hostmetrics reads host-wide operating-system counters. It does not
// substitute panel or core process memory for the host's memory usage.
package hostmetrics

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

type Snapshot struct {
	SampledAt   time.Time `json:"sampled_at"`
	CPUPercent  *float64  `json:"cpu_percent"`
	CPUCount    int       `json:"cpu_count"`
	LoadOne     *float64  `json:"load_one"`
	MemoryTotal *uint64   `json:"memory_total"`
	MemoryUsed  *uint64   `json:"memory_used"`
	DiskTotal   *uint64   `json:"disk_total"`
	DiskUsed    *uint64   `json:"disk_used"`
}

type cpuTimes struct{ total, idle uint64 }
type Sampler struct {
	mu       sync.Mutex
	previous *cpuTimes
	cached   *Snapshot
}

// Sample caches concurrent viewers for one second; the first CPU observation
// has no utilization value because there is no elapsed interval yet.
func (s *Sampler) Sample(path string) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if s.cached != nil && now.Sub(s.cached.SampledAt) < time.Second {
		return s.cached
	}
	snapshot, cpu := readHost(path)
	if snapshot == nil {
		return nil
	}
	snapshot.SampledAt = now
	if cpu != nil && s.previous != nil && cpu.total > s.previous.total && cpu.idle >= s.previous.idle {
		total, idle := cpu.total-s.previous.total, cpu.idle-s.previous.idle
		if idle <= total {
			value := 100 * float64(total-idle) / float64(total)
			snapshot.CPUPercent = &value
		}
	}
	s.previous = cpu
	s.cached = snapshot
	return snapshot
}

// Guest time is included in user/nice already; sum only the first eight values.
func parseCPU(raw string) *cpuTimes {
	line, _, _ := strings.Cut(raw, "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return nil
	}
	result := cpuTimes{}
	for i, value := range fields[1:min(9, len(fields))] {
		number, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil
		}
		if ^uint64(0)-result.total < number {
			return nil
		}
		result.total += number
		if i == 3 || i == 4 {
			result.idle += number
		}
	}
	return &result
}

func parseMemory(raw string) (*uint64, *uint64) {
	values := map[string]uint64{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[2] != "kB" {
			continue
		}
		if fields[0] != "MemTotal:" && fields[0] != "MemAvailable:" {
			continue
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || n > ^uint64(0)/1024 {
			return nil, nil
		}
		values[fields[0]] = n * 1024
	}
	total, ok := values["MemTotal:"]
	available, known := values["MemAvailable:"]
	if !ok || !known || total == 0 || available > total {
		return nil, nil
	}
	used := total - available
	return &total, &used
}
