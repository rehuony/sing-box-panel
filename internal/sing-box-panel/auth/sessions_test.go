// SPDX-License-Identifier: GPL-3.0-or-later
package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestSessionsPersistAcrossDatabaseReopenAndExpire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	app := NewSessions(db, fixedCredentials{email: testutil.AdminEmail, hash: testutil.PasswordHash})
	now := time.Now().UTC().Truncate(time.Second)
	app.now = func() time.Time { return now }
	first, err := app.Login(t.Context(), LoginInput{Email: "  ADMIN@EXAMPLE.COM ", Password: testutil.AdminPassword}, "")
	if err != nil {
		t.Fatal(err)
	}
	firstLoginAt := now
	now = now.Add(24 * time.Hour)
	later, err := app.Login(t.Context(), LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, "")
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := app.Login(t.Context(), LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Logout(t.Context(), revoked.Token); err != nil {
		t.Fatal(err)
	}
	if !first.ExpiresAt.Equal(firstLoginAt.Add(7*24*time.Hour)) || !later.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatal("incorrect fixed lifetimes")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app = NewSessions(db, fixedCredentials{email: testutil.AdminEmail, hash: testutil.PasswordHash})
	app.now = func() time.Time { return now }
	for _, session := range []AuthSession{first, later} {
		restored, err := app.CurrentSession(t.Context(), session.Token)
		if err != nil || restored.CSRFToken != session.CSRFToken || !restored.ExpiresAt.Equal(session.ExpiresAt) {
			t.Fatal("session did not survive actual database reopen", err)
		}
	}
	if _, err := app.CurrentSession(t.Context(), revoked.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("revoked session restored", err)
	}
	now = first.ExpiresAt.Add(-time.Second)
	active, err := app.CurrentSession(t.Context(), first.Token)
	if err != nil || !active.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatal("session expired early or activity extended expiration", err)
	}
	now = first.ExpiresAt
	if _, err := app.CurrentSession(t.Context(), first.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("seven-day session did not expire", err)
	}
	if _, err := app.CurrentSession(t.Context(), later.Token); err != nil {
		t.Fatal(err)
	}
	now = later.ExpiresAt
	if _, err := app.CurrentSession(t.Context(), later.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("later seven-day session did not expire", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app = NewSessions(db, fixedCredentials{email: testutil.AdminEmail, hash: testutil.PasswordHash})
	app.now = func() time.Time { return now }
	if _, err := app.CurrentSession(t.Context(), first.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("expired session restored", err)
	}
}

type fixedCredentials struct{ email, hash string }

func (credentials fixedCredentials) CurrentCredentials(context.Context) (string, string, error) {
	return credentials.email, credentials.hash, nil
}

func (credentials fixedCredentials) WithCredentials(_ context.Context, action func(string, string) error) error {
	return action(credentials.email, credentials.hash)
}
