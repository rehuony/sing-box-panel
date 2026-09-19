// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package hostmetrics

func readHost(string) (*Snapshot, *cpuTimes) { return nil, nil }
