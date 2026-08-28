// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) newID(prefix string) (string, error) {
	raw := make([]byte, 16)
	n, err := application.random(raw)
	if err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	if n != len(raw) {
		return "", fmt.Errorf("generate %s id: short random read", prefix)
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}

func snapshot(value store.CanonicalRevision) CanonicalSnapshot {
	return CanonicalSnapshot{
		ID:            value.ID,
		Sequence:      value.Sequence,
		ParentID:      value.ParentID,
		SchemaVersion: value.SchemaVersion,
		Document:      append(json.RawMessage(nil), value.Document...),
		DocumentJSON:  string(value.Document),
		SHA256:        value.SHA256,
		CreatedAt:     value.CreatedAt,
	}
}
