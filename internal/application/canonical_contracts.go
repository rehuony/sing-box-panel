// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"encoding/json"
	"time"
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
