// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

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
	user, err := database.CreateSubscriptionUser(ctx, SubscriptionUser{
		ID: "user-token", Name: "token user", Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	original, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-original", UserID: user.ID, Label: "original", Enabled: true,
		TokenSHA256: testTokenDigest("original-secret"),
		ExpiresAt:   &expires, CreatedAt: now.Add(time.Second),
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
		ID: "token-duplicate", UserID: user.ID, Label: "duplicate", Enabled: true,
		TokenSHA256: original.TokenSHA256, CreatedAt: now.Add(2 * time.Second),
	}); !errors.Is(err, ErrSubscriptionTokenExists) {
		t.Fatalf("duplicate token digest error = %v", err)
	}

	rotationAt := now.Add(2 * time.Minute)
	newExpiry := now.Add(2 * time.Hour)
	rotation, err := database.RotateSubscriptionToken(ctx, original.ID, SubscriptionToken{
		ID: "token-rotated", UserID: user.ID, Label: original.Label, Enabled: true,
		TokenSHA256: testTokenDigest("rotated-secret"),
		ExpiresAt:   &newExpiry,
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
		ID: "token-rollback", UserID: user.ID, Label: "rollback", Enabled: true,
		TokenSHA256: testTokenDigest("rollback-secret"),
		CreatedAt:   now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.RotateSubscriptionToken(ctx, rollbackCandidate.ID, SubscriptionToken{
		ID: "token-collision", UserID: user.ID, Label: rollbackCandidate.Label, Enabled: true,
		TokenSHA256: rotation.Created.TokenSHA256,
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
	if err := database.DeleteSubscriptionUser(ctx, user.ID, user.UpdatedAt); err != nil {
		t.Fatalf("delete user with owned tokens: %v", err)
	}
	if _, err := database.GetSubscriptionToken(ctx, rollbackCandidate.ID); !errors.Is(err, ErrSubscriptionTokenNotFound) {
		t.Fatalf("token survived owner deletion: %v", err)
	}
	if _, err := database.GetSubscriptionUser(ctx, user.ID); !errors.Is(err, ErrSubscriptionUserNotFound) {
		t.Fatalf("user survived deletion: %v", err)
	}
}

func TestSubscriptionNamesAndTokenMetadataAreStrict(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 11, 0, 0, 0, time.UTC)
	user, err := database.CreateSubscriptionUser(ctx, SubscriptionUser{
		ID: "user-strict", Name: "strict user", Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
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
		ID: "token-uppercase", UserID: user.ID, Label: "uppercase", Enabled: true,
		TokenSHA256: strings.ToUpper(testTokenDigest("secret")), CreatedAt: now,
	}); err == nil {
		t.Fatal("uppercase token digest was accepted")
	}
	past := now.Add(-time.Second)
	if _, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-past", UserID: user.ID, Label: "past", Enabled: true,
		TokenSHA256: testTokenDigest("past"), ExpiresAt: &past, CreatedAt: now,
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
			CreatedAt: now,
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
			Format: SubscriptionFormatLoon, Enabled: true,
			CreatedAt: now.Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatalf("create enabled channel %d: %v", index, err)
		}
	}
	if _, err := database.CreateSubscriptionChannel(ctx, SubscriptionChannel{
		ID: "enabled-overflow", Name: "enabled overflow", Format: SubscriptionFormatLoon,
		Enabled: true, CreatedAt: now.Add(time.Hour),
	}); !errors.Is(err, ErrSubscriptionLimitExceeded) {
		t.Fatalf("enabled overflow error = %v", err)
	}
}

func testTokenDigest(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func TestSubscriptionSecretDigestMismatchIsAtomic(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	_, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-bad-secret", Label: "bad", TokenSHA256: testTokenDigest("expected"), Secret: "different", Enabled: true,
	})
	if err == nil {
		t.Fatal("mismatched secret accepted")
	}
	if _, err := database.GetSubscriptionToken(ctx, "token-bad-secret"); !errors.Is(err, ErrSubscriptionTokenNotFound) {
		t.Fatalf("failed insert persisted: %v", err)
	}
	if _, err := database.SubscriptionTokenSecret(ctx, "token-bad-secret"); !errors.Is(err, ErrSubscriptionTokenNotFound) {
		t.Fatalf("failed secret persisted: %v", err)
	}
}

func TestSubscriptionDigestOnlySecretIsUnavailable(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	created, err := database.CreateSubscriptionToken(ctx, SubscriptionToken{
		ID: "token-digest-only", Label: "old", TokenSHA256: testTokenDigest("legacy-token"), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SubscriptionTokenSecret(ctx, created.ID); !errors.Is(err, ErrSubscriptionTokenSecretUnavailable) {
		t.Fatalf("digest-only secret read: %v", err)
	}
	if _, err := database.GetSubscriptionToken(ctx, created.ID); err != nil {
		t.Fatalf("old token was modified: %v", err)
	}
}
