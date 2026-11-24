// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"github.com/rehuony/sing-box-panel/internal/settingsfile"
)

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
	data, err := Read(path)
	if err != nil {
		return "", err
	}
	return settingsfile.DataDirOf(path, data)
}
