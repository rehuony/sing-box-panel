// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

func readSettings(path string, target any) error {
	data, err := Read(path)
	if err != nil {
		return err
	}
	return decodeSettings(path, data, target)
}

func decodeSettings(path string, data []byte, target any) error {
	if err := jsonstrict.Decode(data, MaximumBytes, target); err != nil {
		return fmt.Errorf("parse settings %q: %w", path, err)
	}
	return nil
}

// LoadDataDir reads only the instance location. Unrelated runtime fields are
// ignored, but malformed, ambiguous, or oversized JSON is still rejected.
func LoadDataDir(path string) (string, error) {
	configured, err := ConfiguredDataDir(path)
	if err != nil {
		return "", err
	}
	return ActiveDataDir(path, configured)
}

// ConfiguredDataDir reads the requested location, including a pending change.
func ConfiguredDataDir(path string) (string, error) {
	var fields map[string]json.RawMessage
	if err := readSettings(path, &fields); err != nil {
		return "", err
	}
	var dataDir string
	if err := json.Unmarshal(fields["data_dir"], &dataDir); err != nil {
		return "", fmt.Errorf("validate settings %q: data_dir must be a non-empty path: %w", path, err)
	}
	return resolveDataDir(path, dataDir)
}

func resolveDataDir(path, dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" || strings.ContainsRune(dataDir, 0) {
		return "", fmt.Errorf("validate settings %q: data_dir must be a non-empty path without NUL bytes", path)
	}
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(filepath.Dir(path), dataDir)
	}
	resolved, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("resolve settings %q data_dir: %w", path, err)
	}
	return resolved, nil
}

// LoadTrafficQuota reads the shared file quota only when a metrics query needs
// it, without validating unrelated runtime fields.
func LoadTrafficQuota(path string) (*int64, error) {
	value, err := LoadTrafficAccounting(path)
	return value.QuotaGiB, err
}

// LoadTrafficAccounting reads quota and period from one file snapshot without
// making read-only metrics commands depend on unrelated runtime settings.
func LoadTrafficAccounting(path string) (Traffic, error) {
	var fields map[string]json.RawMessage
	if err := readSettings(path, &fields); err != nil {
		return Traffic{}, err
	}
	var traffic map[string]json.RawMessage
	if raw, exists := fields["traffic"]; exists {
		if err := json.Unmarshal(raw, &traffic); err != nil {
			return Traffic{}, fmt.Errorf("parse settings %q traffic: %w", path, err)
		}
	}
	value := Traffic{PeriodMonths: 1}
	for field, destination := range map[string]any{"quota_gib": &value.QuotaGiB, "period_months": &value.PeriodMonths} {
		if raw, exists := traffic[field]; exists {
			if err := json.Unmarshal(raw, destination); err != nil {
				return Traffic{}, fmt.Errorf("parse settings %q traffic.%s: %w", path, field, err)
			}
		}
	}
	if err := ValidateTrafficQuota(value.QuotaGiB); err != nil {
		return Traffic{}, err
	}
	if value.PeriodMonths < 1 || value.PeriodMonths > 120 {
		return Traffic{}, errors.New("traffic.period_months must be between 1 and 120")
	}
	return value, nil
}

// ValidateTrafficQuota ensures a quota can be safely converted from GiB to bytes.
func ValidateTrafficQuota(quota *int64) error {
	if quota != nil && (*quota < 0 || *quota > (1<<63-1)/(1<<30)) {
		return errors.New("traffic.quota_gib must be null or a non-negative value representable in bytes")
	}
	return nil
}
