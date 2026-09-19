package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSubscriptionKeyMigrationRetainsLegacyScopeAndUsage(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := migrationFiles.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		string(initial), fmt.Sprintf("PRAGMA application_id = %d", ApplicationID),
		"PRAGMA user_version = 1", "INSERT INTO schema_migrations VALUES(1, '0001_initial', '2026-09-01T00:00:00Z')",
		`INSERT INTO subscription_users(id,name,enabled,created_at,updated_at) VALUES('legacy','Legacy',1,'2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`,
		`INSERT INTO subscription_user_node_grants(user_id,node_key,created_at) VALUES('legacy','local:existing','2026-09-01T00:00:00Z')`,
		fmt.Sprintf(`INSERT INTO subscription_tokens(id,user_id,label,token_sha256,successful_request_count,body_response_count,bytes_served,created_at) VALUES('old','legacy','Old','%s',9,7,420,'2026-09-01T00:00:00Z')`, testTokenDigest("legacy")),
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	key, err := upgraded.GetSubscriptionToken(ctx, "old")
	if err != nil || key.UserID != "legacy" || key.DownloadLimit != nil || key.BodyResponseCount != 7 || key.SuccessfulRequestCount != 9 || key.BytesServed != 420 || key.TokenSHA256 != testTokenDigest("legacy") {
		t.Fatalf("migration changed legacy key: %+v %v", key, err)
	}
	var grant string
	if err := upgraded.db.QueryRowContext(ctx, `SELECT node_key FROM subscription_user_node_grants WHERE user_id='legacy'`).Scan(&grant); err != nil || grant != "local:existing" {
		t.Fatalf("grant: %s %v", grant, err)
	}
}

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
