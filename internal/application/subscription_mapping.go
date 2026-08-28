// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) newSubscriptionTokenSecret() (string, string, error) {
	raw := make([]byte, subscriptionTokenEntropyBytes)
	n, err := application.random(raw)
	if err != nil {
		return "", "", fmt.Errorf("generate subscription token: %w", err)
	}
	if n != len(raw) {
		return "", "", errors.New("generate subscription token: short random read")
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(digest[:]), nil
}

func (application *Application) nextSubscriptionUpdateTime(expected time.Time) time.Time {
	now := application.now().UTC()
	if !now.After(expected.UTC()) {
		return expected.UTC().Add(time.Nanosecond)
	}
	return now
}

func applicationSubscriptionChannel(value store.SubscriptionChannel) SubscriptionChannel {
	return SubscriptionChannel{
		ID: value.ID, Name: value.Name, Format: value.Format, PublicHost: value.PublicHost,
		Config: append(json.RawMessage(nil), value.Config...), Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func applicationSubscriptionChannelSummary(value store.SubscriptionChannelSummary) SubscriptionChannelSummary {
	return SubscriptionChannelSummary{
		ID: value.ID, Name: value.Name, Format: value.Format, PublicHost: value.PublicHost, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func applicationSubscriptionSource(value store.SubscriptionSource) SubscriptionSource {
	return SubscriptionSource{
		ID: value.ID, Name: value.Name, SourceKind: value.SourceKind,
		Config:           append(json.RawMessage(nil), value.Config...),
		CurrentVersionID: value.CurrentVersionID,
		Enabled:          value.Enabled, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func applicationSubscriptionSourceSummary(value store.SubscriptionSourceSummary) SubscriptionSourceSummary {
	return SubscriptionSourceSummary{
		ID: value.ID, Name: value.Name, SourceKind: value.SourceKind,
		HasVersion: value.HasVersion, Enabled: value.Enabled,
		CurrentVersionID: value.CurrentVersionID,
		CreatedAt:        value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func applicationSubscriptionSourceVersion(value store.SubscriptionSourceVersion, includeRaw bool) SubscriptionSourceVersion {
	result := SubscriptionSourceVersion{
		ID: value.ID, SourceID: value.SourceID, Format: value.Format,
		NormalizedNodes: append(json.RawMessage(nil), value.NormalizedNodes...),
		Diagnostics:     append(json.RawMessage(nil), value.Diagnostics...), SHA256: value.SHA256,
		FetchedAt: value.FetchedAt, CreatedAt: value.CreatedAt,
	}
	if includeRaw {
		result.RawBody = append([]byte(nil), value.RawBody...)
	}
	return result
}

func applicationSubscriptionToken(value store.SubscriptionToken, at time.Time) SubscriptionToken {
	return SubscriptionToken{
		ID: value.ID, UserID: value.UserID, Label: value.Label, Enabled: value.Enabled,
		ExpiresAt: cloneTime(value.ExpiresAt), RevokedAt: cloneTime(value.RevokedAt),
		SuccessfulRequestCount: value.SuccessfulRequestCount, BodyResponseCount: value.BodyResponseCount,
		BytesServed: value.BytesServed, LastUsedAt: cloneTime(value.LastUsedAt),
		CreatedAt: value.CreatedAt, Active: value.Active(at),
	}
}

func applicationSubscriptionUser(value store.SubscriptionUser) SubscriptionUser {
	return SubscriptionUser{
		ID: value.ID, Name: value.Name, Description: value.Description, Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func storeSubscriptionCursor(value *SubscriptionCursor) *store.CreatedAtCursor {
	if value == nil {
		return nil
	}
	return &store.CreatedAtCursor{CreatedAt: value.CreatedAt, ID: strings.TrimSpace(value.ID)}
}

func applicationSubscriptionCursor(value *store.CreatedAtCursor) *SubscriptionCursor {
	if value == nil {
		return nil
	}
	return &SubscriptionCursor{CreatedAt: value.CreatedAt, ID: value.ID}
}
