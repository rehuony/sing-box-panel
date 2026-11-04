// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
)

// The views of a node's runtime that the management API returns.

type EnabledCore struct {
	CoreArtifactID   string `json:"core_artifact_id"`
	ExactCoreVersion string `json:"exact_core_version"`
}

type RunningCore struct {
	PID                int       `json:"pid"`
	ProcessStartToken  string    `json:"process_start_token"`
	ExactCoreVersion   string    `json:"exact_core_version"`
	CoreArtifactID     string    `json:"core_artifact_id"`
	ArchiveSHA256      string    `json:"archive_sha256"`
	BinarySHA256       string    `json:"binary_sha256"`
	ActivationBundleID string    `json:"activation_bundle_id"`
	StartedAt          time.Time `json:"started_at"`
}

type RuntimeStatus struct {
	EnabledCore               *EnabledCore `json:"enabled_core,omitempty"`
	LoadedCanonicalRevisionID string       `json:"loaded_canonical_revision_id,omitempty"`
	DesiredRunning            bool         `json:"desired_running"`
	DesiredBundleID           string       `json:"desired_bundle_id,omitempty"`
	AppliedBundleID           string       `json:"applied_bundle_id,omitempty"`
	RollbackBundleID          string       `json:"rollback_bundle_id,omitempty"`
	TargetGeneration          int64        `json:"target_generation"`
	ObservationState          string       `json:"observation_state"`
	Running                   *RunningCore `json:"running,omitempty"`
}

// ActivationSummary identifies an applied startup artifact and its bundle.
type ActivationSummary struct {
	StartupArtifactID   string `json:"startup_artifact_id"`
	CanonicalRevisionID string `json:"canonical_revision_id"`
	ExactCoreVersion    string `json:"exact_core_version"`
	CoreArtifactID      string `json:"core_artifact_id"`
	ConfigSHA256        string `json:"config_sha256"`
	ActivationBundleID  string `json:"activation_bundle_id"`
	ActivationSHA256    string `json:"activation_sha256"`
	MonitoringTier      string `json:"monitoring_tier"`
}

type StartupArtifactSummary struct {
	ID                  string     `json:"id"`
	CanonicalRevisionID string     `json:"canonical_revision_id"`
	ExactCoreVersion    string     `json:"exact_core_version"`
	CoreArtifactID      string     `json:"core_artifact_id"`
	ConfigSHA256        string     `json:"config_sha256"`
	State               string     `json:"state"`
	CheckedAt           *time.Time `json:"checked_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type StartupArtifactCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type StartupArtifactPage struct {
	Items []StartupArtifactSummary `json:"items"`
	Next  *StartupArtifactCursor   `json:"next,omitempty"`
}

// StartupArtifactListRequest filters a node's startup artifacts.
type StartupArtifactListRequest struct {
	CanonicalRevisionID string
	ExactCoreVersion    string
	CoreArtifactID      string
	State               string
	Cursor              *StartupArtifactCursor
	Limit               int
}

type RuntimeResponse struct {
	Status     RuntimeStatus           `json:"status"`
	Activation *ActivationSummary      `json:"activation,omitempty"`
	Startup    *StartupArtifactSummary `json:"startup_artifact,omitempty"`
}

type CompiledConfigurationArtifact struct {
	ID                  string `json:"id"`
	CanonicalRevisionID string `json:"canonical_revision_id"`
	ExactCoreVersion    string `json:"exact_core_version"`
	CoreArtifactID      string `json:"core_artifact_id"`
	ConfigSHA256        string `json:"config_sha256"`
	State               string `json:"state"`
}

type ConfigurationCompile struct {
	Support  nodeconfig.ConfigurationSupport `json:"support"`
	Artifact CompiledConfigurationArtifact   `json:"artifact"`
}

// PathQuery browses one directory of a node's filesystem.
type PathQuery struct {
	Path       string
	Search     string
	ShowHidden bool
	Offset     int
	Limit      int
}

type PathEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Symlink   bool   `json:"symlink"`
	Available bool   `json:"available"`
}

type PathPage struct {
	Path          string      `json:"path"`
	Parent        string      `json:"parent"`
	RequestedPath string      `json:"requested_path"`
	Fallback      bool        `json:"fallback"`
	Items         []PathEntry `json:"items"`
	Total         int         `json:"total"`
	Offset        int         `json:"offset"`
	Limit         int         `json:"limit"`
}

type PathSelection struct {
	Path    string `json:"path"`
	Parent  string `json:"parent"`
	Kind    string `json:"kind"`
	Exists  bool   `json:"exists"`
	Symlink bool   `json:"symlink"`
}

type CoreLogFile struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	Deletable bool      `json:"deletable"`
}

type CoreLogChunk struct {
	File       string `json:"file"`
	Text       string `json:"text"`
	NextOffset int64  `json:"next_offset"`
	Size       int64  `json:"size"`
	Generation string `json:"generation"`
	Reset      bool   `json:"reset,omitempty"`
}
