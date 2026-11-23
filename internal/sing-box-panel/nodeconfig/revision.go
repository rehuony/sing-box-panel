// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodeconfig owns the editable sing-box configuration file, its
// canonical revisions, and the Schema support and preview for a selected core.
package nodeconfig

import (
	"encoding/json"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

type CanonicalSnapshot struct {
	ID            string          `json:"id"`
	Sequence      int64           `json:"sequence"`
	ParentID      string          `json:"parent_id,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Document      json.RawMessage `json:"document"`
	DocumentJSON  string          `json:"document_json"`
	SHA256        string          `json:"sha256"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Snapshot converts a stored canonical revision to its API view.
func Snapshot(value store.CanonicalRevision) CanonicalSnapshot {
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
