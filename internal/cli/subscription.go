// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"

	"github.com/rehuony/sing-box-panel/internal/store"
)

const (
	maximumSubscriptionCLIInputBytes  int64 = 5 << 20
	maximumSubscriptionCLIConfigBytes int64 = 64 << 10
)

type subscriptionChannelWriteInput struct {
	Name       *string                   `json:"name"`
	Format     *store.SubscriptionFormat `json:"format"`
	PublicHost *string                   `json:"public_host"`
	Config     json.RawMessage           `json:"config,omitempty"`
	Enabled    *bool                     `json:"enabled"`
}

type subscriptionSourceCreateInput struct {
	Name       *string                       `json:"name"`
	SourceKind *store.SubscriptionSourceKind `json:"source_kind"`
	Config     json.RawMessage               `json:"config,omitempty"`
	Enabled    *bool                         `json:"enabled"`
}

type subscriptionSourceWriteInput struct {
	Name       *string                       `json:"name"`
	SourceKind *store.SubscriptionSourceKind `json:"source_kind"`
	Config     json.RawMessage               `json:"config,omitempty"`
	Enabled    *bool                         `json:"enabled"`
}
