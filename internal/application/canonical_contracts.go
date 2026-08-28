// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"encoding/json"
	"time"
)

type CoreVersionResolution struct {
	ExactVersion string           `json:"exact_version"`
	Source       string           `json:"source"`
	Running      *RuntimeIdentity `json:"running,omitempty"`
}

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

type CanonicalChange struct {
	Operation string `json:"op"`
	Path      string `json:"path"`
	ValueJSON string `json:"value_json,omitempty"`
}

type CanonicalSave struct {
	Revision CanonicalSnapshot `json:"revision"`
	TaskID   string            `json:"task_id,omitempty"`
	NoChange bool              `json:"no_change"`
}

type CanonicalValue struct {
	Revision CanonicalSnapshot `json:"revision"`
	Pointer  string            `json:"pointer"`
	Value    any               `json:"value"`
}

type CanonicalRevisionPage struct {
	Items []CanonicalSnapshot `json:"items"`
	Next  *int64              `json:"next_before_sequence,omitempty"`
}

type DiffValue struct {
	Present bool `json:"present"`
	Value   any  `json:"value,omitempty"`
}

type CanonicalDiffEntry struct {
	Path string    `json:"path"`
	From DiffValue `json:"from"`
	To   DiffValue `json:"to"`
}

type CanonicalRevisionDiff struct {
	From    CanonicalSnapshot    `json:"from"`
	To      CanonicalSnapshot    `json:"to"`
	Changes []CanonicalDiffEntry `json:"changes"`
}
