// SPDX-License-Identifier: GPL-3.0-or-later

// Package releasegate contains objective checks that distinguish a development
// build from an artifact that is eligible for a generally available release.
package releasegate

import (
	"strconv"
	"strings"

	sqlite3 "modernc.org/sqlite/lib"
)

const MinimumSQLiteVersion = "3.53.4"

type SQLiteStatus struct {
	Current string `json:"current"`
	Minimum string `json:"minimum"`
	Ready   bool   `json:"ready"`
}

func SQLite() SQLiteStatus {
	current := sqlite3.SQLITE_VERSION
	return SQLiteStatus{
		Current: current,
		Minimum: MinimumSQLiteVersion,
		Ready:   compareVersions(current, MinimumSQLiteVersion) >= 0,
	}
}

func compareVersions(left, right string) int {
	leftParts, leftOK := numericVersion(left)
	rightParts, rightOK := numericVersion(right)
	if !leftOK || !rightOK {
		return -1
	}
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}

func numericVersion(value string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(value, ".")
	if len(parts) != len(result) {
		return result, false
	}
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return [3]int{}, false
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return [3]int{}, false
		}
		result[index] = parsed
	}
	return result, true
}
