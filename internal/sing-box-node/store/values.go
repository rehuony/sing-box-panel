// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"time"
)

type rowScanner interface {
	Scan(...any) error
}

func formatTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// CloneTime copies an optional record time, so a view does not share it with
// the record.
func CloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
