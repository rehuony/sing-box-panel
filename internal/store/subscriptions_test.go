// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionChannelCRUDValidationOrderingAndCAS(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 8, 0, 0, 0, time.UTC)

	beta, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "channel-beta", Name: "beta", Format: SubscriptionFormatLoon,
		Config:  json.RawMessage(` { "exclude_tags": ["private"] } `),
		Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "channel-alpha", Name: "alpha", Format: SubscriptionFormatSingBox,
		Config: json.RawMessage(`{}`), Enabled: true, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(beta.Config) != `{"exclude_tags":["private"]}` || !beta.CreatedAt.Equal(beta.UpdatedAt) {
		t.Fatalf("normalized beta = %+v", beta)
	}

	channels, err := database.ListSubscriptionChannels(ctx, SubscriptionChannelListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(channels.Items) != 2 || channels.Items[0].ID != alpha.ID || channels.Items[1].ID != beta.ID {
		t.Fatalf("ordered channels = %+v", channels)
	}
	reloaded, err := database.GetSubscriptionChannel(ctx, alpha.ID)
	if err != nil || string(reloaded.Config) != `{}` {
		t.Fatalf("defensive channel config = %s, err=%v", reloaded.Config, err)
	}

	if _, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "channel-other", Name: alpha.Name, Format: SubscriptionFormatMihomo,
		CreatedAt: now.Add(2 * time.Second),
	}); !errors.Is(err, ErrSubscriptionChannelExists) {
		t.Fatalf("duplicate channel name error = %v", err)
	}
	if _, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "channel-invalid", Name: "bad", Format: "clash", CreatedAt: now,
	}); err == nil {
		t.Fatal("unsupported channel format was accepted")
	}
	for name, config := range map[string]json.RawMessage{
		"unknown field":   json.RawMessage(`{"unknown":true}`),
		"duplicate key":   json.RawMessage(`{"exclude_tags":[],"exclude_tags":[]}`),
		"duplicate value": json.RawMessage(`{"exclude_tags":["same","same"]}`),
		"invalid type":    json.RawMessage(`{"exclude_types":["VMess"]}`),
		"null root":       json.RawMessage(`null`),
		"null list":       json.RawMessage(`{"exclude_tags":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
				ID: "invalid-" + strings.ReplaceAll(name, " ", "-"), Name: "invalid " + name,
				Format: SubscriptionFormatSingBox, Config: config, CreatedAt: now,
			})
			if err == nil {
				t.Fatalf("invalid config %s was accepted", config)
			}
		})
	}

	updated, err := database.UpdateSubscriptionChannel(ctx, UpdateSubscriptionChannelInput{
		ID: alpha.ID, Name: "alpha-renamed", Format: SubscriptionFormatMihomo,
		Config: json.RawMessage(`{"exclude_types":["direct"]}`), Enabled: false,
		ExpectedUpdatedAt: alpha.UpdatedAt, UpdatedAt: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "alpha-renamed" || updated.Format != SubscriptionFormatMihomo || updated.Enabled {
		t.Fatalf("updated channel = %+v", updated)
	}
	if _, err := database.UpdateSubscriptionChannel(ctx, UpdateSubscriptionChannelInput{
		ID: alpha.ID, Name: "stale", Format: SubscriptionFormatLoon, Enabled: true,
		ExpectedUpdatedAt: alpha.UpdatedAt, UpdatedAt: now.Add(4 * time.Second),
	}); !errors.Is(err, ErrSubscriptionConflict) {
		t.Fatalf("stale channel update error = %v", err)
	}
	if err := database.DeleteSubscriptionChannel(ctx, alpha.ID, alpha.UpdatedAt); !errors.Is(err, ErrSubscriptionConflict) {
		t.Fatalf("stale channel delete error = %v", err)
	}
	if err := database.DeleteSubscriptionChannel(ctx, alpha.ID, updated.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSubscriptionChannel(ctx, alpha.ID); !errors.Is(err, ErrSubscriptionChannelNotFound) {
		t.Fatalf("deleted channel lookup error = %v", err)
	}
}

func TestSubscriptionSourceCRUDSnapshotAndCAS(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)

	zulu, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-zulu", Name: "zulu", SourceKind: SubscriptionSourceRemote,
		Config: json.RawMessage(`{"url":"https://example.test/sub"}`), Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-alpha", Name: "alpha", SourceKind: SubscriptionSourceLocal,
		Config: json.RawMessage(`{}`), LatestSnapshot: json.RawMessage(`[{"tag":"a"}]`),
		Enabled: true, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := database.ListSubscriptionSources(ctx, SubscriptionSourceListFilter{})
	if err != nil || len(sources.Items) != 2 || sources.Items[0].ID != alpha.ID || sources.Items[1].ID != zulu.ID {
		t.Fatalf("ordered sources = %+v, err=%v", sources, err)
	}
	if string(alpha.LatestSnapshot) != `[{"tag":"a"}]` {
		t.Fatalf("initial snapshot = %s", alpha.LatestSnapshot)
	}

	if _, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-duplicate", Name: zulu.Name, SourceKind: SubscriptionSourceLocal,
		CreatedAt: now.Add(2 * time.Second),
	}); !errors.Is(err, ErrSubscriptionSourceExists) {
		t.Fatalf("duplicate source error = %v", err)
	}
	if _, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-bad-kind", Name: "bad kind", SourceKind: "file", CreatedAt: now,
	}); err == nil {
		t.Fatal("invalid source kind was accepted")
	}
	if _, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-bad-config", Name: "bad config", SourceKind: SubscriptionSourceRemote,
		Config: json.RawMessage(`{"url":"one","url":"two"}`), CreatedAt: now,
	}); err == nil {
		t.Fatal("ambiguous source config was accepted")
	}

	snapshotted, err := database.UpdateSubscriptionSourceSnapshot(ctx, UpdateSubscriptionSourceSnapshotInput{
		ID: zulu.ID, LatestSnapshot: json.RawMessage(` { "nodes": [1, 2] } `),
		ExpectedUpdatedAt: zulu.UpdatedAt, UpdatedAt: now.Add(3 * time.Second),
	})
	if err != nil || string(snapshotted.LatestSnapshot) != `{"nodes":[1,2]}` {
		t.Fatalf("snapshot update = %+v, err=%v", snapshotted, err)
	}
	if _, err := database.UpdateSubscriptionSourceSnapshot(ctx, UpdateSubscriptionSourceSnapshotInput{
		ID: zulu.ID, LatestSnapshot: json.RawMessage(`null`),
		ExpectedUpdatedAt: snapshotted.UpdatedAt, UpdatedAt: now.Add(4 * time.Second),
	}); err == nil {
		t.Fatal("null source snapshot was accepted")
	}
	if _, err := database.UpdateSubscriptionSource(ctx, UpdateSubscriptionSourceInput{
		ID: zulu.ID, Name: "stale", SourceKind: SubscriptionSourceRemote, Config: json.RawMessage(`{}`),
		Enabled: true, ExpectedUpdatedAt: zulu.UpdatedAt, UpdatedAt: now.Add(5 * time.Second),
	}); !errors.Is(err, ErrSubscriptionConflict) {
		t.Fatalf("stale source update error = %v", err)
	}

	updated, err := database.UpdateSubscriptionSource(ctx, UpdateSubscriptionSourceInput{
		ID: zulu.ID, Name: "remote", SourceKind: SubscriptionSourceLocal, Config: json.RawMessage(`{"mode":"manual"}`),
		Enabled: false, ExpectedUpdatedAt: snapshotted.UpdatedAt, UpdatedAt: now.Add(5 * time.Second),
	})
	if err != nil || updated.Enabled || updated.SourceKind != SubscriptionSourceLocal ||
		string(updated.LatestSnapshot) != string(snapshotted.LatestSnapshot) {
		t.Fatalf("updated source = %+v, err=%v", updated, err)
	}
	updated.Config[0] = 'x'
	reloaded, err := database.GetSubscriptionSource(ctx, zulu.ID)
	if err != nil || string(reloaded.Config) != `{"mode":"manual"}` {
		t.Fatalf("defensive source = %+v, err=%v", reloaded, err)
	}
	if err := database.DeleteSubscriptionSource(ctx, zulu.ID, updated.UpdatedAt); err != nil {
		t.Fatal(err)
	}
}

func TestSubscriptionTokenHashRotationRevocationExpiryAndGlobalScope(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	channel, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "channel-token", Name: "token", Format: SubscriptionFormatSingBox,
		Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	original, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-original", TokenSHA256: testTokenDigest("original-secret"),
		ExpiresAt: &expires, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	var persistedDigest string
	if err := database.db.QueryRowContext(
		ctx,
		`SELECT token_sha256 FROM subscription_tokens WHERE id = ?`,
		original.ID,
	).Scan(&persistedDigest); err != nil {
		t.Fatal(err)
	}
	if persistedDigest != testTokenDigest("original-secret") || strings.Contains(persistedDigest, "original-secret") {
		t.Fatalf("persisted token material = %q, want only SHA-256", persistedDigest)
	}
	if !original.Active(now.Add(time.Minute)) || original.Active(expires) {
		t.Fatalf("token activity was not expiry-exclusive: %+v", original)
	}
	active, err := database.FindActiveSubscriptionToken(ctx, original.TokenSHA256, now.Add(time.Minute))
	if err != nil || active.ID != original.ID {
		t.Fatalf("active lookup = %+v, err=%v", active, err)
	}
	if _, err := database.FindActiveSubscriptionToken(ctx, original.TokenSHA256, expires); !errors.Is(err, ErrSubscriptionTokenInactive) {
		t.Fatalf("expired token lookup error = %v", err)
	}
	if _, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-duplicate", TokenSHA256: original.TokenSHA256, CreatedAt: now.Add(2 * time.Second),
	}); !errors.Is(err, ErrSubscriptionTokenExists) {
		t.Fatalf("duplicate token digest error = %v", err)
	}

	rotationAt := now.Add(2 * time.Minute)
	newExpiry := now.Add(2 * time.Hour)
	rotation, err := database.RotateSubscriptionToken(ctx, original.ID, SubscriptionToken{
		ID: "token-rotated", TokenSHA256: testTokenDigest("rotated-secret"),
		ExpiresAt: &newExpiry,
	}, rotationAt)
	if err != nil {
		t.Fatal(err)
	}
	if rotation.Revoked.RevokedAt == nil || !rotation.Revoked.RevokedAt.Equal(rotationAt) ||
		!rotation.Created.Active(rotationAt) {
		t.Fatalf("rotation = %+v", rotation)
	}
	if _, err := database.FindActiveSubscriptionToken(ctx, original.TokenSHA256, rotationAt); !errors.Is(err, ErrSubscriptionTokenInactive) {
		t.Fatalf("rotated old token error = %v", err)
	}
	if found, err := database.FindActiveSubscriptionToken(ctx, rotation.Created.TokenSHA256, rotationAt); err != nil || found.ID != rotation.Created.ID {
		t.Fatalf("rotated new token = %+v, err=%v", found, err)
	}

	rollbackCandidate, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-rollback", TokenSHA256: testTokenDigest("rollback-secret"),
		CreatedAt: now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.RotateSubscriptionToken(ctx, rollbackCandidate.ID, SubscriptionToken{
		ID: "token-collision", TokenSHA256: rotation.Created.TokenSHA256,
	}, now.Add(4*time.Minute)); !errors.Is(err, ErrSubscriptionTokenExists) {
		t.Fatalf("colliding rotation error = %v", err)
	}
	unchanged, err := database.GetSubscriptionToken(ctx, rollbackCandidate.ID)
	if err != nil || unchanged.RevokedAt != nil {
		t.Fatalf("failed rotation was not rolled back: %+v, err=%v", unchanged, err)
	}

	revoked, err := database.RevokeSubscriptionToken(ctx, rotation.Created.ID, now.Add(5*time.Minute))
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("revoke = %+v, err=%v", revoked, err)
	}
	repeated, err := database.RevokeSubscriptionToken(ctx, rotation.Created.ID, now.Add(6*time.Minute))
	if err != nil || repeated.RevokedAt == nil || !repeated.RevokedAt.Equal(*revoked.RevokedAt) {
		t.Fatalf("idempotent revoke = %+v, err=%v", repeated, err)
	}

	tokens, err := database.ListSubscriptionTokens(ctx, SubscriptionTokenListFilter{})
	if err != nil || len(tokens.Items) != 3 || tokens.Items[0].ID != rollbackCandidate.ID {
		t.Fatalf("ordered tokens = %+v, err=%v", tokens, err)
	}
	if err := database.DeleteSubscriptionChannel(ctx, channel.ID, channel.UpdatedAt); err != nil {
		t.Fatalf("delete channel with global token: %v", err)
	}
}

func TestSubscriptionNamesAndTokenMetadataAreStrict(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 11, 0, 0, 0, time.UTC)
	for _, name := range []string{"", " leading", "trailing ", "line\nbreak", strings.Repeat("x", 129)} {
		_, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
			ID: "channel-" + testTokenDigest(name)[:8], Name: name,
			Format: SubscriptionFormatLoon, CreatedAt: now,
		})
		if err == nil {
			t.Fatalf("invalid name %q was accepted", name)
		}
	}
	if _, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-uppercase", TokenSHA256: strings.ToUpper(testTokenDigest("secret")), CreatedAt: now,
	}); err == nil {
		t.Fatal("uppercase token digest was accepted")
	}
	past := now.Add(-time.Second)
	if _, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-past", TokenSHA256: testTokenDigest("past"), ExpiresAt: &past, CreatedAt: now,
	}); err == nil {
		t.Fatal("token expiring before creation was accepted")
	}
	if _, err := database.FindActiveSubscriptionToken(ctx, testTokenDigest("missing"), now); !errors.Is(err, ErrSubscriptionTokenNotFound) {
		t.Fatalf("missing token error = %v", err)
	}
}

func TestSubscriptionChannelPaginationAndEnabledLimit(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"channel-a", "channel-c", "channel-b"} {
		if _, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
			ID: id, Name: id, Format: SubscriptionFormatSingBox,
			Config: json.RawMessage(`{}`), CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := database.ListSubscriptionChannels(ctx, SubscriptionChannelListFilter{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.Items[0].ID != "channel-c" ||
		first.Items[1].ID != "channel-b" || first.Next == nil {
		t.Fatalf("first page = %+v, err=%v", first, err)
	}
	second, err := database.ListSubscriptionChannels(ctx, SubscriptionChannelListFilter{Cursor: first.Next, Limit: 2})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "channel-a" || second.Next != nil {
		t.Fatalf("second page = %+v, err=%v", second, err)
	}

	for index := 0; index < MaximumEnabledSubscriptionChannels; index++ {
		if _, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
			ID: fmt.Sprintf("enabled-%03d", index), Name: fmt.Sprintf("enabled %03d", index),
			Format: SubscriptionFormatLoon, Config: json.RawMessage(`{}`), Enabled: true,
			CreatedAt: now.Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatalf("create enabled channel %d: %v", index, err)
		}
	}
	if _, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "enabled-overflow", Name: "enabled overflow", Format: SubscriptionFormatLoon,
		Config: json.RawMessage(`{}`), Enabled: true, CreatedAt: now.Add(time.Hour),
	}); !errors.Is(err, ErrSubscriptionLimitExceeded) {
		t.Fatalf("enabled overflow error = %v", err)
	}
}

func testTokenDigest(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
