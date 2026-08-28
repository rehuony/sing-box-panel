// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

const subscriptionTokenEntropyBytes = 32

var ErrSubscriptionPreviewArtifactState = errors.New("startup artifact cannot be previewed")

type SubscriptionChannel struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Format     store.SubscriptionFormat `json:"format"`
	PublicHost string                   `json:"public_host"`
	Config     json.RawMessage          `json:"config"`
	Enabled    bool                     `json:"enabled"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
}

type SubscriptionChannelSummary struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Format     store.SubscriptionFormat `json:"format"`
	PublicHost string                   `json:"public_host"`
	Enabled    bool                     `json:"enabled"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
}

type CreateSubscriptionChannelRequest struct {
	Name       string                   `json:"name"`
	Format     store.SubscriptionFormat `json:"format"`
	PublicHost string                   `json:"public_host"`
	Config     json.RawMessage          `json:"config"`
	Enabled    bool                     `json:"enabled"`
}

type UpdateSubscriptionChannelRequest struct {
	Name              string                   `json:"name"`
	Format            store.SubscriptionFormat `json:"format"`
	PublicHost        string                   `json:"public_host"`
	Config            json.RawMessage          `json:"config"`
	Enabled           bool                     `json:"enabled"`
	ExpectedUpdatedAt time.Time                `json:"expected_updated_at"`
}

type SubscriptionSource struct {
	ID               string                       `json:"id"`
	Name             string                       `json:"name"`
	SourceKind       store.SubscriptionSourceKind `json:"source_kind"`
	Config           json.RawMessage              `json:"config"`
	CurrentVersionID string                       `json:"current_version_id,omitempty"`
	Enabled          bool                         `json:"enabled"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

type SubscriptionSourceSummary struct {
	ID               string                       `json:"id"`
	Name             string                       `json:"name"`
	SourceKind       store.SubscriptionSourceKind `json:"source_kind"`
	HasVersion       bool                         `json:"has_version"`
	CurrentVersionID string                       `json:"current_version_id,omitempty"`
	Enabled          bool                         `json:"enabled"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

type CreateSubscriptionSourceRequest struct {
	Name       string                       `json:"name"`
	SourceKind store.SubscriptionSourceKind `json:"source_kind"`
	Config     json.RawMessage              `json:"config"`
	Enabled    bool                         `json:"enabled"`
}

type UpdateSubscriptionSourceRequest struct {
	Name              string                       `json:"name"`
	SourceKind        store.SubscriptionSourceKind `json:"source_kind"`
	Config            json.RawMessage              `json:"config"`
	Enabled           bool                         `json:"enabled"`
	ExpectedUpdatedAt time.Time                    `json:"expected_updated_at"`
}

type SubscriptionSourceVersion struct {
	ID              string          `json:"id"`
	SourceID        string          `json:"source_id"`
	Format          string          `json:"format"`
	RawBody         []byte          `json:"raw_body,omitempty"`
	NormalizedNodes json.RawMessage `json:"normalized_nodes"`
	Diagnostics     json.RawMessage `json:"diagnostics"`
	SHA256          string          `json:"sha256"`
	FetchedAt       time.Time       `json:"fetched_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

type SubscriptionSourceVersionPage struct {
	Items []SubscriptionSourceVersion `json:"items"`
	Next  *SubscriptionCursor         `json:"next,omitempty"`
}

type CreateSubscriptionSourceVersionRequest struct {
	Format            subscription.SourceFormat
	RawBody           []byte
	ExpectedUpdatedAt time.Time
	FetchedAt         time.Time
}

type SubscriptionSourceVersionSave struct {
	Source  SubscriptionSource        `json:"source"`
	Version SubscriptionSourceVersion `json:"version"`
}

type SubscriptionUser struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateSubscriptionUserRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type UpdateSubscriptionUserRequest struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Enabled           bool      `json:"enabled"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
}

type SubscriptionUserPage struct {
	Items []SubscriptionUser  `json:"items"`
	Next  *SubscriptionCursor `json:"next,omitempty"`
}

type SubscriptionUserGrants struct {
	User   SubscriptionUser `json:"user"`
	Grants []string         `json:"grants"`
}

type SubscriptionNodeSummary struct {
	Key        string `json:"key"`
	SourceID   string `json:"source_id"`
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Credential string `json:"credential,omitempty"`
}

type SubscriptionNodeCatalog struct {
	AppliedBundleID string                              `json:"applied_bundle_id"`
	Nodes           []SubscriptionNodeSummary           `json:"nodes"`
	Diagnostics     []subscription.ConversionDiagnostic `json:"diagnostics"`
}

// SubscriptionToken deliberately omits both plaintext and token_sha256. The
// public plaintext is returned only by create/rotate result types.
type SubscriptionToken struct {
	ID                     string     `json:"id"`
	UserID                 string     `json:"user_id"`
	Label                  string     `json:"label"`
	Enabled                bool       `json:"enabled"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	RevokedAt              *time.Time `json:"revoked_at,omitempty"`
	SuccessfulRequestCount int64      `json:"successful_request_count"`
	BodyResponseCount      int64      `json:"body_response_count"`
	BytesServed            int64      `json:"bytes_served"`
	LastUsedAt             *time.Time `json:"last_used_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	Active                 bool       `json:"active"`
}

type CreateSubscriptionTokenRequest struct {
	UserID    string     `json:"user_id"`
	Label     string     `json:"label"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type SubscriptionCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type SubscriptionListRequest struct {
	Cursor *SubscriptionCursor
	Limit  int
}

type SubscriptionChannelPage struct {
	Items []SubscriptionChannelSummary `json:"items"`
	Next  *SubscriptionCursor          `json:"next,omitempty"`
}

type SubscriptionSourcePage struct {
	Items []SubscriptionSourceSummary `json:"items"`
	Next  *SubscriptionCursor         `json:"next,omitempty"`
}

type SubscriptionTokenPage struct {
	Items []SubscriptionToken `json:"items"`
	Next  *SubscriptionCursor `json:"next,omitempty"`
}

type CreatedSubscriptionToken struct {
	Metadata SubscriptionToken `json:"metadata"`
	Token    string            `json:"token"`
}

type SubscriptionTokenRotation struct {
	Revoked SubscriptionToken `json:"revoked"`
	Created SubscriptionToken `json:"created"`
	Token   string            `json:"token"`
}

type SubscriptionPreview struct {
	UserID              string                     `json:"user_id"`
	AppliedBundleID     string                     `json:"applied_bundle_id"`
	Channel             SubscriptionChannel        `json:"channel"`
	StartupArtifactID   string                     `json:"startup_artifact_id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	ArtifactState       store.StartupArtifactState `json:"artifact_state"`
	Result              subscription.RenderResult  `json:"result"`
}
