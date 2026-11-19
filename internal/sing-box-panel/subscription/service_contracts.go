// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const subscriptionTokenEntropyBytes = 32

type SubscriptionChannel struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Format    store.SubscriptionFormat `json:"format"`
	Config    ChannelConfig            `json:"config"`
	Enabled   bool                     `json:"enabled"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type SubscriptionChannelSummary struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Format    store.SubscriptionFormat `json:"format"`
	Enabled   bool                     `json:"enabled"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type CreateSubscriptionChannelRequest struct {
	Name    string                   `json:"name"`
	Format  store.SubscriptionFormat `json:"format"`
	Config  json.RawMessage          `json:"config"`
	Enabled bool                     `json:"enabled"`
}

type UpdateSubscriptionChannelRequest struct {
	Name              string                   `json:"name"`
	Format            store.SubscriptionFormat `json:"format"`
	Config            json.RawMessage          `json:"config"`
	Enabled           bool                     `json:"enabled"`
	ExpectedUpdatedAt time.Time                `json:"expected_updated_at"`
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

// SubscriptionToken deliberately omits both plaintext and token_sha256. The
// plaintext is available only through explicit secret reads and create/rotate.
type SubscriptionToken struct {
	DownloadLimit          *int64     `json:"download_limit,omitempty"`
	ID                     string     `json:"id"`
	UserID                 string     `json:"user_id,omitempty"`
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
	DownloadLimit *int64     `json:"download_limit,omitempty"`
	UserID        string     `json:"user_id,omitempty"`
	Label         string     `json:"label"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type SubscriptionTokenSecret struct {
	Token string `json:"token"`
}

type SubscriptionCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type SubscriptionListRequest struct {
	Cursor *SubscriptionCursor
	Limit  int
	Offset int // Token lists also support numbered pages.
}

type SubscriptionChannelPage struct {
	Items []SubscriptionChannelSummary `json:"items"`
	Next  *SubscriptionCursor          `json:"next,omitempty"`
}

type SubscriptionTokenPage struct {
	Items []SubscriptionToken `json:"items"`
	Next  *SubscriptionCursor `json:"next,omitempty"`
	Total int                 `json:"total"`
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
	UserID           string                    `json:"user_id"`
	Channel          SubscriptionChannel       `json:"channel"`
	NodePublications []NodePublicationSummary  `json:"node_publications"`
	Result           SubscriptionPreviewResult `json:"result"`
}

// SubscriptionPreviewResult is an authenticated projection of rendered bytes and
// node diagnostics. Public delivery never serializes the enriched diagnostics.
type SubscriptionPreviewResult struct {
	Format      RenderFormat        `json:"format"`
	MediaType   string              `json:"media_type"`
	Content     []byte              `json:"content"`
	NodeCount   int                 `json:"node_count"`
	Diagnostics []PreviewDiagnostic `json:"diagnostics"`
}
