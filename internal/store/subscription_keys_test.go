package store

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSubscriptionDownloadQuotaIsAtomicAndRotationRetainsUsage(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	now := time.Now().UTC()
	limit := int64(8)
	key, err := db.CreateSubscriptionToken(ctx, SubscriptionToken{ID: "key", Label: "Shared", Enabled: true, TokenSHA256: testTokenDigest("shared"), DownloadLimit: &limit, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSubscriptionTokenUse(ctx, key.ID, now, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSubscriptionTokenUse(ctx, key.ID, now, 20); err != nil {
		t.Fatal(err)
	}
	rotated, err := db.RotateSubscriptionToken(ctx, key.ID, SubscriptionToken{ID: "replacement", Label: key.Label, Enabled: true, TokenSHA256: testTokenDigest("replacement")}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Created.BodyResponseCount != 1 || rotated.Created.DownloadLimit == nil || *rotated.Created.DownloadLimit != limit {
		t.Fatalf("rotation reset quota: %+v", rotated)
	}
	if err := db.RecordSubscriptionTokenUse(ctx, key.ID, now.Add(time.Second), 20); !errors.Is(err, ErrSubscriptionTokenInactive) {
		t.Fatalf("revoked key accepted: %v", err)
	}
	var wg sync.WaitGroup
	results := make([]error, 24)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = db.RecordSubscriptionTokenUse(ctx, rotated.Created.ID, now.Add(2*time.Second), 20)
		}()
	}
	wg.Wait()
	success := 0
	for _, err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrSubscriptionTokenInactive) {
			t.Fatal(err)
		}
	}
	if success != 7 {
		t.Fatalf("remaining downloads = %d, want 7", success)
	}
	exhausted, err := db.GetSubscriptionToken(ctx, rotated.Created.ID)
	if err != nil || exhausted.Active(now) || exhausted.BodyResponseCount != limit || exhausted.SuccessfulRequestCount != limit+1 || exhausted.BytesServed != limit*20 {
		t.Fatalf("usage: %+v %v", exhausted, err)
	}
}

func TestSubscriptionDownloadRechecksExpiryAndUserState(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	now := time.Now().UTC()
	expires := now.Add(time.Minute)
	key, err := db.CreateSubscriptionToken(ctx, SubscriptionToken{ID: "expiring", Label: "Expiring", Enabled: true, TokenSHA256: testTokenDigest("expiry"), CreatedAt: now, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSubscriptionTokenUse(ctx, key.ID, expires, 50); !errors.Is(err, ErrSubscriptionTokenInactive) {
		t.Fatalf("expiry race: %v", err)
	}
	user, err := db.CreateSubscriptionUser(ctx, SubscriptionUser{ID: "owner", Name: "Owner", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	key, err = db.CreateSubscriptionToken(ctx, SubscriptionToken{ID: "scoped", UserID: user.ID, Label: "Scoped", Enabled: true, TokenSHA256: testTokenDigest("scoped"), CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `UPDATE subscription_users SET enabled=0 WHERE id=?`, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSubscriptionTokenUse(ctx, key.ID, now, 50); !errors.Is(err, ErrSubscriptionTokenInactive) {
		t.Fatalf("disabled owner race: %v", err)
	}
}
