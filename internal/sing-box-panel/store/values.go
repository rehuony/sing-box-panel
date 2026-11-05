// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

type rowScanner interface {
	Scan(...any) error
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
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

func compactJSON(value json.RawMessage, defaultValue string) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(defaultValue)
	}
	if !json.Valid(value) {
		return nil, errors.New("invalid JSON")
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, value); err != nil {
		return nil, fmt.Errorf("compact JSON: %w", err)
	}
	return append(json.RawMessage(nil), compacted.Bytes()...), nil
}

func validProcessStartToken(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func canonicalJSONArrayWithLimit(value json.RawMessage, fallback string, maximum int64) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(fallback)
	}
	var array []any
	if err := jsonstrict.Decode(value, maximum, &array); err != nil || array == nil {
		return nil, errors.New("value must be one strict JSON array")
	}
	encoded, err := json.Marshal(array)
	return bytes.Clone(encoded), err
}
