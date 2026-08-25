// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestApplicationSubscriptionChannelAndSourceCRUD(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)

	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "public", Format: store.SubscriptionFormatMihomo,
		Config: json.RawMessage(`{"exclude_tags":["private"]}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(channel.ID, "channel_") || string(channel.Config) != `{"exclude_tags":["private"]}` {
		t.Fatalf("created channel = %+v", channel)
	}
	loaded, err := app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || loaded.ID != channel.ID {
		t.Fatalf("loaded channel = %+v, err=%v", loaded, err)
	}
	listed, err := app.ListSubscriptionChannels(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != channel.ID {
		t.Fatalf("listed channels = %+v, err=%v", listed, err)
	}
	updated, err := app.UpdateSubscriptionChannel(ctx, channel.ID, UpdateSubscriptionChannelRequest{
		Name: "renamed", Format: store.SubscriptionFormatLoon, Config: json.RawMessage(`{}`),
		Enabled: false, ExpectedUpdatedAt: channel.UpdatedAt,
	})
	if err != nil || updated.Enabled || !updated.UpdatedAt.After(channel.UpdatedAt) {
		t.Fatalf("updated channel = %+v, err=%v", updated, err)
	}
	if _, err := app.UpdateSubscriptionChannel(ctx, channel.ID, UpdateSubscriptionChannelRequest{
		Name: "stale", Format: store.SubscriptionFormatLoon, ExpectedUpdatedAt: channel.UpdatedAt,
	}); !errors.Is(err, store.ErrSubscriptionConflict) {
		t.Fatalf("stale application channel update error = %v", err)
	}

	source, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "upstream", SourceKind: store.SubscriptionSourceRemote,
		Config: json.RawMessage(`{"url":"https://example.test/sub"}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(source.ID, "source_") || len(source.LatestSnapshot) != 0 {
		t.Fatalf("created source = %+v", source)
	}
	snapshotted, err := app.UpdateSubscriptionSourceSnapshot(ctx, source.ID, UpdateSubscriptionSourceSnapshotRequest{
		LatestSnapshot:    json.RawMessage(`{"nodes":[{"tag":"upstream"}]}`),
		ExpectedUpdatedAt: source.UpdatedAt,
	})
	if err != nil || !bytes.Contains(snapshotted.LatestSnapshot, []byte(`"upstream"`)) {
		t.Fatalf("snapshotted source = %+v, err=%v", snapshotted, err)
	}
	updatedSource, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "local copy", SourceKind: store.SubscriptionSourceLocal,
		Config: json.RawMessage(`{}`), Enabled: false,
		ExpectedUpdatedAt: snapshotted.UpdatedAt,
	})
	if err != nil || updatedSource.Enabled || updatedSource.SourceKind != store.SubscriptionSourceLocal ||
		len(updatedSource.LatestSnapshot) == 0 {
		t.Fatalf("updated source = %+v, err=%v", updatedSource, err)
	}
	sources, err := app.ListSubscriptionSources(ctx)
	if err != nil || len(sources) != 1 || sources[0].ID != source.ID {
		t.Fatalf("listed sources = %+v, err=%v", sources, err)
	}
	if err := app.DeleteSubscriptionSource(ctx, source.ID, updatedSource.UpdatedAt); err != nil {
		t.Fatal(err)
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
		Name: "token channel", Format: store.SubscriptionFormatSingBox, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	expires := app.now().Add(time.Hour)
	created, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{
		ChannelID: channel.ID, ExpiresAt: &expires,
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
	listed, err := app.ListSubscriptionTokens(ctx)
	if err != nil || len(listed) != 1 {
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
	if err := app.DeleteSubscriptionChannel(ctx, channel.ID, channel.UpdatedAt); !errors.Is(err, store.ErrSubscriptionChannelInUse) {
		t.Fatalf("referenced application channel delete error = %v", err)
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
			app := newApplication(database)
			app.random = test.random
			if _, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{}); err == nil {
				t.Fatal("token creation accepted failed entropy")
			}
			tokens, err := database.ListSubscriptionTokens(ctx)
			if err != nil || len(tokens) != 0 {
				t.Fatalf("tokens after entropy failure = %+v, err=%v", tokens, err)
			}
		})
	}
}

func TestRenderSubscriptionPreviewUsesReadyOrFrozenStaleArtifact(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)
	now := app.now().UTC()
	core := applicationTestCore("core-subscription", "1.13.19", 'a', 'b', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	canonicalSave, err := app.ReplaceCanonical(ctx, "", canonical.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	startupBytes := []byte(`{
      "outbounds":[
        {"type":"shadowsocks","tag":"hidden","server":"hidden.example","server_port":443,"method":"aes-128-gcm","password":"hidden-secret"},
        {"type":"shadowsocks","tag":"public","server":"public.example","server_port":8443,"method":"aes-256-gcm","password":"public-secret"}
      ]
    }`)
	ready, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-subscription-ready", Kind: store.StartupArtifactManual,
		CanonicalRevisionID: canonicalSave.Revision.ID, ExactCoreVersion: core.ExactVersion,
		RendererVersion: "manual-v1", CoreArtifactID: core.ID, ConfigBytes: startupBytes,
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err = database.CompleteStartupArtifactCheck(ctx, ready.ID, true, nil, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "filtered", Format: store.SubscriptionFormatSingBox,
		Config: json.RawMessage(`{"exclude_tags":["hidden"]}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	preview, err := app.RenderSubscriptionPreview(ctx, ready.ID, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ArtifactState != store.StartupArtifactReady || preview.Result.NodeCount != 1 ||
		!bytes.Contains(preview.Result.Content, []byte(`"tag":"public"`)) ||
		bytes.Contains(preview.Result.Content, []byte(`"tag":"hidden"`)) {
		t.Fatalf("ready preview = %+v, content=%s", preview, preview.Result.Content)
	}
	stale, err := database.MarkStartupArtifactStale(ctx, ready.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := app.RenderSubscriptionPreview(ctx, stale.ID, channel.ID)
	if err != nil || frozen.ArtifactState != store.StartupArtifactStale ||
		!bytes.Equal(frozen.Result.Content, preview.Result.Content) {
		t.Fatalf("stale preview = %+v, err=%v", frozen, err)
	}

	pending, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-subscription-pending", Kind: store.StartupArtifactManual,
		CanonicalRevisionID: canonicalSave.Revision.ID, ExactCoreVersion: core.ExactVersion,
		RendererVersion: "manual-v1", CoreArtifactID: core.ID, ConfigBytes: startupBytes,
		CreatedAt: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RenderSubscriptionPreview(ctx, pending.ID, channel.ID); !errors.Is(err, ErrSubscriptionPreviewArtifactState) {
		t.Fatalf("pending preview error = %v", err)
	}
}

func newSubscriptionTestApplication(database *store.Store) *Application {
	app := newApplication(database)
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
