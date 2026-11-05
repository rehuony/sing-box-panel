// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func TestTrafficPeriodStartsOnTheNodesResetDay(t *testing.T) {
	day := func(value string) time.Time {
		parsed, err := time.Parse(time.DateOnly, value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	for _, test := range []struct {
		at                 time.Time
		months, resetDay   int
		wantStart, wantEnd string
	}{
		{day("2026-11-05"), 1, 1, "2026-11-01", "2026-12-01"},
		{day("2026-11-05"), 1, 15, "2026-10-15", "2026-11-15"},
		{day("2026-11-15"), 1, 15, "2026-11-15", "2026-12-15"},
		{day("2026-11-15").Add(-time.Nanosecond), 1, 15, "2026-10-15", "2026-11-15"},
		{day("2027-01-03"), 1, 28, "2026-12-28", "2027-01-28"},
		// Longer periods align to months counted from January 1970.
		{day("2026-11-05"), 3, 1, "2026-10-01", "2027-01-01"},
		{day("2026-10-10"), 3, 15, "2026-07-15", "2026-10-15"},
	} {
		start, end, err := TrafficPeriod(test.at, store.NodeTrafficSettings{PeriodMonths: test.months, ResetDay: test.resetDay})
		if err != nil || !start.Equal(day(test.wantStart)) || !end.Equal(day(test.wantEnd)) {
			t.Fatalf("period of %s (%d months from day %d) = %s – %s, %v; want %s – %s",
				test.at, test.months, test.resetDay, start, end, err, test.wantStart, test.wantEnd)
		}
	}
	if _, _, err := TrafficPeriod(day("2026-11-05"), store.NodeTrafficSettings{PeriodMonths: 1, ResetDay: 29}); err == nil {
		t.Fatal("a reset day after the 28th was accepted")
	}
}
