// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestSessionsPersistAcrossDatabaseReopenAndExpire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	cfg := settingsFileFixture(t, settings.Defaults())
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	app := FromStoreWithSettings(db, cfg)
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
	app = FromStoreWithSettings(db, cfg)
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
	app = FromStoreWithSettings(db, cfg)
	if _, err := app.CurrentSession(t.Context(), first.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("expired session restored", err)
	}
}

func TestCredentialChangesSerializeWithLoginsAndRevokeAllSessions(t *testing.T) {
	app := panelFileApp(t)
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	sessions := make(chan AuthSession, 8)
	failures := make(chan error, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			<-start
			session, err := app.Login(t.Context(), LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, "")
			if err != nil {
				failures <- err
			} else {
				sessions <- session
			}
		})
	}
	close(start)
	saved, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Credentials: &CredentialsWrite{Email: new("new@example.com")}})
	if err != nil || !saved.ReauthenticationRequired {
		t.Fatal("email-only change failed", err)
	}
	workers.Wait()
	close(sessions)
	close(failures)
	for err := range failures {
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	for session := range sessions {
		if _, err := app.CurrentSession(t.Context(), session.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
			t.Fatal("concurrent login left old session usable", err)
		}
	}
	fresh, err := app.Login(t.Context(), LoginInput{Email: "new@example.com", Password: testutil.AdminPassword}, "")
	if err != nil {
		t.Fatal("email-only change did not preserve password", err)
	}
	same, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: saved.Settings.Revision, Preferences: saved.Settings.Preferences, Credentials: &CredentialsWrite{Email: new(" NEW@example.com "), NewPassword: testutil.AdminPassword}})
	if err != nil || same.ReauthenticationRequired {
		t.Fatal("unchanged credentials revoked session", err)
	}
	if _, err := app.CurrentSession(t.Context(), fresh.Token); err != nil {
		t.Fatal(err)
	}
	backup, err := app.ExportPanelBackup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if backup.Version != 2 {
		t.Fatal("backup version")
	}
	// Returning to another account through restore must revoke the newer session.
	raw, err := settings.Load(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	raw.Auth.Email = testutil.AdminEmail
	backup.PanelSettings, err = encodeSettings(raw, backup.PanelSettings, true)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := app.ConfigurationFile(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RestorePanelBackup(t.Context(), PanelRestoreRequest{Backup: backup, SettingsRevision: same.Settings.Revision, ConfigurationRevision: configuration.Revision})
	if err != nil || !result.ReauthenticationRequired {
		t.Fatal("restore credentials", err)
	}
	if _, err := app.CurrentSession(t.Context(), fresh.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("restore did not revoke", err)
	}
}
