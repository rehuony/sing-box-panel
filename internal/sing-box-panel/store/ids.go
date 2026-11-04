// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/hex"
	"fmt"
)

// NewID returns a record identifier: prefix, an underscore and 16 bytes read
// from random in hexadecimal.
func NewID(random func([]byte) (int, error), prefix string) (string, error) {
	raw := make([]byte, 16)
	n, err := random(raw)
	if err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	if n != len(raw) {
		return "", fmt.Errorf("generate %s id: short random read", prefix)
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}
