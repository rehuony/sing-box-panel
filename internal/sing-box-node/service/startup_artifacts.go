// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// StartupArtifactSummary deliberately excludes compiled configuration bytes,
// which may contain credentials.
type StartupArtifactSummary struct {
	ID                  string                     `json:"id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	CoreArtifactID      string                     `json:"core_artifact_id"`
	ConfigSHA256        string                     `json:"config_sha256"`
	State               store.StartupArtifactState `json:"state"`
	CheckedAt           *time.Time                 `json:"checked_at,omitempty"`
	CreatedAt           time.Time                  `json:"created_at"`
}
