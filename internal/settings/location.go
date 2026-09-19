// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

func readSettings(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read settings %q: %w", path, err)
	}
	if err := jsonstrict.Decode(data, maxSettingsBytes, target); err != nil {
		return fmt.Errorf("parse settings %q: %w", path, err)
	}
	return nil
}

// LoadDataDir reads only the instance location. Unrelated runtime fields are
// ignored, but malformed, ambiguous, or oversized JSON is still rejected.
func LoadDataDir(path string) (string, error) {
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

// LoadTrafficQuota reads the optional bootstrap quota only when a metrics
// query needs it and no persisted panel preferences override it.
func LoadTrafficQuota(path string) (*int64, error) {
	var fields map[string]json.RawMessage
	if err := readSettings(path, &fields); err != nil {
		return nil, err
	}
	var traffic map[string]json.RawMessage
	if raw, exists := fields["traffic"]; exists {
		if err := json.Unmarshal(raw, &traffic); err != nil {
			return nil, fmt.Errorf("parse settings %q traffic: %w", path, err)
		}
	}
	var quota *int64
	if raw, exists := traffic["quota_gib"]; exists {
		if err := json.Unmarshal(raw, &quota); err != nil {
			return nil, fmt.Errorf("parse settings %q traffic.quota_gib: %w", path, err)
		}
	}
	if err := ValidateTrafficQuota(quota); err != nil {
		return nil, fmt.Errorf("validate settings %q: %w", path, err)
	}
	return quota, nil
}

// ValidateTrafficQuota ensures a quota can be safely converted from GiB to bytes.
func ValidateTrafficQuota(quota *int64) error {
	if quota != nil && (*quota < 0 || *quota > (1<<63-1)/(1<<30)) {
		return errors.New("traffic.quota_gib must be null or a non-negative value representable in bytes")
	}
	return nil
}
