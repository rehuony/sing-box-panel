// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// emptyChannel is a channel configuration without groups.
var emptyChannel = json.RawMessage(`{"groups":[],"default_exit":{"kind":"direct"}}`)

func TestApplicationSubscriptionChannelAndSourceCRUD(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)

	source, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "upload", Kind: store.SubscriptionSourceFile, Filter: &SourceFilter{},
		Content: new("socks5://user:pass@socks.example:1080#Socks\nhttp://http.example:8080#Web\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(source.ID, "source_") || source.NodeCount != 2 || source.ContentFormat != SourceFormatShareLinks ||
		source.Format != SourceFormatAuto || source.RefreshMinutes != nil || source.NextRefreshAt != nil {
		t.Fatalf("created source = %+v", source)
	}
	pool, err := app.SubscriptionNodes(ctx)
	if err != nil || len(pool.Items) != 2 || pool.Items[0].DisplayName != "Socks" || pool.Items[1].Origin.SourceName != "upload" {
		t.Fatalf("pool = %+v, %v", pool, err)
	}

	config := json.RawMessage(`{"incompatible_nodes":"skip","groups":[{"id":"main","name":"Main","enabled":true,"type":"select",
		"members":[{"kind":"node","id":"` + pool.Items[1].ID + `"},{"kind":"builtin","id":"direct"}],
		"rules":[{"sort_index":0,"id":"r","enabled":true,"kind":"domain","value":"example.org","exit":{"kind":"node","id":"` + pool.Items[0].ID + `"}}]}],
		"default_exit":{"kind":"group","id":"main"},"template":{"format":"mihomo","content":"mode: rule\n"}}`)
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "public", Format: store.SubscriptionFormatMihomo, Config: config, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(channel.Config)
	var want, got any
	_ = json.Unmarshal(config, &want)
	_ = json.Unmarshal(encoded, &got)
	if !strings.HasPrefix(channel.ID, "channel_") || !reflect.DeepEqual(got, want) {
		t.Fatalf("created channel = %s", encoded)
	}
	loaded, err := app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || !reflect.DeepEqual(loaded, channel) {
		t.Fatalf("loaded channel = %+v, err=%v", loaded, err)
	}
	listed, err := app.ListSubscriptionChannels(ctx, SubscriptionListRequest{})
	if err != nil || len(listed.Items) != 1 || listed.Items[0].ID != channel.ID {
		t.Fatalf("listed channels = %+v, err=%v", listed, err)
	}
	if _, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "dangling", Format: store.SubscriptionFormatMihomo, Enabled: true,
		Config: json.RawMessage(`{"groups":[],"default_exit":{"kind":"node","id":"sn_missing"}}`),
	}); !errors.As(err, new(*store.SubscriptionReferenceError)) {
		t.Fatalf("a channel named a node outside the pool: %v", err)
	}
	updated, err := app.UpdateSubscriptionChannel(ctx, channel.ID, UpdateSubscriptionChannelRequest{
		Name: "renamed", Format: store.SubscriptionFormatLoon, Config: emptyChannel,
		Enabled: false, ExpectedUpdatedAt: channel.UpdatedAt,
	})
	if err != nil || updated.Enabled || len(updated.Config.Groups) != 0 || !updated.UpdatedAt.After(channel.UpdatedAt) {
		t.Fatalf("updated channel = %+v, err=%v", updated, err)
	}
	if _, err := app.UpdateSubscriptionChannel(ctx, channel.ID, UpdateSubscriptionChannelRequest{
		Name: "stale", Format: store.SubscriptionFormatLoon, Config: emptyChannel, ExpectedUpdatedAt: channel.UpdatedAt,
	}); !errors.Is(err, store.ErrSubscriptionConflict) {
		t.Fatalf("stale application channel update error = %v", err)
	}

	// A filter applies to the stored content at once.
	updatedSource, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "uploaded", Filter: &SourceFilter{Exclude: []string{"web"}},
	}, source.UpdatedAt)
	if err != nil || updatedSource.NodeCount != 1 || updatedSource.Name != "uploaded" || !updatedSource.UpdatedAt.After(source.UpdatedAt) {
		t.Fatalf("updated source = %+v, err=%v", updatedSource, err)
	}
	if _, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "uploaded", Filter: &SourceFilter{Include: []string{"/(/"}},
	}, updatedSource.UpdatedAt); !errors.As(err, new(*SourceFieldError)) {
		t.Fatalf("invalid filter error = %v", err)
	}
	if _, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "url", URL: new("https://example.test/sub"), RefreshMinutes: new(0), Filter: &SourceFilter{},
	}, updatedSource.UpdatedAt); !errors.As(err, new(*SourceFieldError)) {
		t.Fatalf("a file source took a URL: %v", err)
	}
	if _, err := app.RefreshSubscriptionSource(ctx, source.ID); !errors.Is(err, ErrSourceKindMismatch) {
		t.Fatalf("a file source refreshed: %v", err)
	}
	sources, err := app.ListSubscriptionSources(ctx, SubscriptionListRequest{})
	if err != nil || len(sources.Items) != 1 || sources.Items[0].ID != source.ID {
		t.Fatalf("listed sources = %+v, err=%v", sources, err)
	}
	if err := app.DeleteSubscriptionSource(ctx, source.ID, source.UpdatedAt); !errors.Is(err, store.ErrSubscriptionConflict) {
		t.Fatalf("stale source delete error = %v", err)
	}
	if err := app.DeleteSubscriptionSource(ctx, source.ID, updatedSource.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if pool, err := app.SubscriptionNodes(ctx); err != nil || len(pool.Items) != 0 {
		t.Fatalf("a deleted source left nodes: %+v, %v", pool, err)
	}
	if err := app.DeleteSubscriptionChannel(ctx, channel.ID, updated.UpdatedAt); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationSubscriptionTokenPlaintextLifecycleDoesNotLeakFromReads(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "token channel", Format: store.SubscriptionFormatSingBox, Config: emptyChannel, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, err := app.CreateSubscriptionUser(ctx, CreateSubscriptionUserRequest{
		Name: "token user", Description: "application token owner", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	expires := app.now().Add(time.Hour)
	created, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{
		UserID: user.ID, Label: "primary", ExpiresAt: &expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(created.Token)
	if err != nil || len(decoded) != subscriptionTokenEntropyBytes {
		t.Fatalf("generated token has %d bytes, err=%v", len(decoded), err)
	}
	if !created.Metadata.Active || !strings.HasPrefix(created.Metadata.ID, "token_") {
		t.Fatalf("created token metadata = %+v", created.Metadata)
	}
	authenticated, err := app.AuthenticateSubscriptionToken(ctx, created.Token)
	if err != nil || authenticated.ID != created.Metadata.ID || !authenticated.Active {
		t.Fatalf("authenticated token = %+v, err=%v", authenticated, err)
	}

	stored, err := database.GetSubscriptionToken(ctx, created.Metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(created.Token))
	if stored.TokenSHA256 != hex.EncodeToString(digest[:]) || stored.TokenSHA256 == created.Token {
		t.Fatalf("stored token digest = %q", stored.TokenSHA256)
	}
	listed, err := app.ListSubscriptionTokens(ctx, SubscriptionListRequest{})
	if err != nil || len(listed.Items) != 1 {
		t.Fatalf("listed tokens = %+v, err=%v", listed, err)
	}
	encodedList, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encodedList, []byte(created.Token)) || bytes.Contains(encodedList, []byte(stored.TokenSHA256)) {
		t.Fatalf("token list leaked secret material: %s", encodedList)
	}

	rotatedExpiry := app.now().Add(2 * time.Hour)
	rotation, err := app.RotateSubscriptionToken(ctx, created.Metadata.ID, &rotatedExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if rotation.Token == "" || rotation.Token == created.Token || rotation.Revoked.Active || !rotation.Created.Active {
		t.Fatalf("rotation = %+v", rotation)
	}
	if _, err := app.AuthenticateSubscriptionToken(ctx, created.Token); !errors.Is(err, store.ErrSubscriptionTokenInactive) {
		t.Fatalf("old plaintext after rotate error = %v", err)
	}
	if token, err := app.AuthenticateSubscriptionToken(ctx, rotation.Token); err != nil || token.ID != rotation.Created.ID {
		t.Fatalf("new plaintext authentication = %+v, err=%v", token, err)
	}
	revoked, err := app.RevokeSubscriptionToken(ctx, rotation.Created.ID)
	if err != nil || revoked.Active || revoked.RevokedAt == nil {
		t.Fatalf("revoked token = %+v, err=%v", revoked, err)
	}
	if _, err := app.AuthenticateSubscriptionToken(ctx, rotation.Token); !errors.Is(err, store.ErrSubscriptionTokenInactive) {
		t.Fatalf("revoked plaintext authentication error = %v", err)
	}
	if err := app.DeleteSubscriptionChannel(ctx, channel.ID, channel.UpdatedAt); err != nil {
		t.Fatalf("delete channel with global token: %v", err)
	}
}

func TestApplicationSubscriptionTokenRotationPreservesState(t *testing.T) {
	for _, test := range []struct {
		name      string
		enabled   bool
		expired   bool
		exhausted bool
	}{
		{name: "enabled", enabled: true},
		{name: "disabled"},
		{name: "expired", enabled: true, expired: true},
		{name: "exhausted", enabled: true, exhausted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			app := newSubscriptionTestApplication(database)
			expires := app.now().Add(time.Hour)
			limit := int64(3)
			if test.exhausted {
				limit = 1
			}
			created, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{
				Label: "Rotatable", ExpiresAt: &expires, DownloadLimit: &limit,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.RecordSubscriptionTokenUse(ctx, created.Metadata.ID, app.now(), 42); err != nil {
				t.Fatal(err)
			}
			if _, err := app.SetSubscriptionTokenEnabled(ctx, created.Metadata.ID, test.enabled); err != nil {
				t.Fatal(err)
			}
			if test.expired {
				app.now = func() time.Time { return expires }
			}
			original, err := app.SubscriptionToken(ctx, created.Metadata.ID)
			if err != nil {
				t.Fatal(err)
			}
			rotation, err := app.RotateSubscriptionToken(ctx, original.ID, nil)
			if err != nil {
				t.Fatalf("rotate %s key: %v", test.name, err)
			}
			replacement := rotation.Created
			if replacement.Enabled != original.Enabled || replacement.Active != original.Active ||
				replacement.Label != original.Label || replacement.UserID != original.UserID ||
				replacement.ExpiresAt == nil || !replacement.ExpiresAt.Equal(expires) ||
				replacement.DownloadLimit == nil || *replacement.DownloadLimit != limit ||
				replacement.BodyResponseCount != 1 || replacement.SuccessfulRequestCount != 1 ||
				replacement.BytesServed != 42 || replacement.LastUsedAt == nil || !replacement.LastUsedAt.Equal(*original.LastUsedAt) {
				t.Fatalf("rotation changed key state: original=%+v replacement=%+v", original, replacement)
			}
			if rotation.Token == "" || rotation.Token == created.Token || replacement.ID == original.ID || rotation.Revoked.RevokedAt == nil {
				t.Fatalf("rotation did not replace and revoke the old key: %+v", rotation)
			}
			if _, err := app.AuthenticateSubscriptionToken(ctx, created.Token); !errors.Is(err, store.ErrSubscriptionTokenInactive) {
				t.Fatalf("old secret authentication error = %v", err)
			}
			var wantAuthError error
			if !original.Active {
				wantAuthError = store.ErrSubscriptionTokenInactive
			}
			if _, err := app.AuthenticateSubscriptionToken(ctx, rotation.Token); !errors.Is(err, wantAuthError) {
				t.Fatalf("replacement authentication error = %v, want %v", err, wantAuthError)
			}
			if _, err := app.RotateSubscriptionToken(ctx, original.ID, nil); !errors.Is(err, store.ErrSubscriptionTokenInactive) {
				t.Fatalf("revoked key rotation error = %v", err)
			}
		})
	}
}

func TestApplicationSubscriptionTokenRandomFailuresDoNotPersist(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name   string
		random func([]byte) (int, error)
	}{
		{name: "error", random: func([]byte) (int, error) { return 0, errors.New("entropy unavailable") }},
		{name: "short", random: func(value []byte) (int, error) { return len(value) - 1, nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			app := newSubscriptionTestService(database)
			app.random = test.random
			if _, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{}); err == nil {
				t.Fatal("token creation accepted failed entropy")
			}
			tokens, err := database.ListSubscriptionTokens(ctx, store.SubscriptionTokenListFilter{})
			if err != nil || len(tokens.Items) != 0 {
				t.Fatalf("tokens after entropy failure = %+v, err=%v", tokens, err)
			}
		})
	}
}

func newSubscriptionTestApplication(database *store.Store) *Service {
	app := newSubscriptionTestService(database)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	sequence := byte(0)
	app.random = func(destination []byte) (int, error) {
		sequence++
		for index := range destination {
			destination[index] = sequence
		}
		return len(destination), nil
	}
	return app
}

func TestSubscriptionSecretPersistsAcrossRestartAndRotation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")
	database, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	app := newSubscriptionTestApplication(database)
	created, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{Label: "share"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	random := app.random
	app = newSubscriptionTestApplication(database)
	app.random = random
	revealed, err := app.SubscriptionTokenSecret(ctx, created.Metadata.ID)
	if err != nil || revealed.Token != created.Token {
		t.Fatal("stored secret did not survive restart")
	}
	rotation, err := app.RotateSubscriptionToken(ctx, created.Metadata.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := app.SubscriptionTokenSecret(ctx, rotation.Created.ID)
	if err != nil || replacement.Token != rotation.Token || replacement.Token == created.Token {
		t.Fatal("rotation secret mismatch")
	}
	if _, err := app.AuthenticateSubscriptionToken(ctx, created.Token); !errors.Is(err, store.ErrSubscriptionTokenInactive) {
		t.Fatal("old token remains active")
	}
	if err := app.DeleteSubscriptionToken(ctx, rotation.Created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SubscriptionTokenSecret(ctx, rotation.Created.ID); !errors.Is(err, store.ErrSubscriptionTokenNotFound) {
		t.Fatalf("deleted secret read: %v", err)
	}
}

// newSubscriptionTestService serves read-only default settings; tests call
// useSettings to read a settings file.
func newSubscriptionTestService(database *store.Store) *Service {
	panelSettings := testSettings(database, "", settings.Settings{})
	return New(database, panelSettings, telemetry.New(database, panelSettings, telemetry.Options{}), Options{})
}

// useSettings serves panelSettings.
func (app *Service) useSettings(panelSettings *settings.Service) {
	app.settings = panelSettings
	app.telemetry = telemetry.New(app.database, panelSettings, telemetry.Options{})
}

func testSettings(database *store.Store, path string, value settings.Settings) *settings.Service {
	return settings.NewService(database, path, value)
}
