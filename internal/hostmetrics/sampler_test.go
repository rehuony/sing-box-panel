// SPDX-License-Identifier: GPL-3.0-or-later
package hostmetrics

import "testing"

func TestCPUExcludesDoubleCountedGuest(t *testing.T) {
	got := parseCPU("cpu 100 20 30 400 50 10 5 5 60 10\ncpu0 0 0 0 0")
	if got == nil || got.total != 620 || got.idle != 450 {
		t.Fatalf("%+v", got)
	}
	for _, raw := range []string{"", "cpu 1 2", "cpu 1 x 2 3", "cpu 18446744073709551615 1 0 0"} {
		if parseCPU(raw) != nil {
			t.Fatal(raw)
		}
	}
}
func TestMemoryRequiresAvailableEvidence(t *testing.T) {
	total, used := parseMemory("MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB")
	if total == nil || *total != 1024000 || *used != 614400 {
		t.Fatal(total, used)
	}
	for _, raw := range []string{"MemTotal: 1000 kB", "MemTotal: 100 kB\nMemAvailable: 400 kB"} {
		if a, b := parseMemory(raw); a != nil || b != nil {
			t.Fatal(raw)
		}
	}
}
