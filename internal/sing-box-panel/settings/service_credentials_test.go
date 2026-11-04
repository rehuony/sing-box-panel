// SPDX-License-Identifier: GPL-3.0-or-later
package settings

import (
	"errors"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestCredentialChangesSerializeWithLoginsAndRevokeAllSessions(t *testing.T) {
	app := panelFileApp(t)
	authSessions := auth.NewSessions(app.database, app)
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	sessions := make(chan auth.AuthSession, 8)
	failures := make(chan error, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			<-start
			session, err := authSessions.Login(t.Context(), auth.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, "")
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
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	for session := range sessions {
		if _, err := authSessions.CurrentSession(t.Context(), session.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
			t.Fatal("concurrent login left old session usable", err)
		}
	}
	fresh, err := authSessions.Login(t.Context(), auth.LoginInput{Email: "new@example.com", Password: testutil.AdminPassword}, "")
	if err != nil {
		t.Fatal("email-only change did not preserve password", err)
	}
	same, err := app.SavePanelSettings(t.Context(), PanelSettingsWrite{Revision: saved.Settings.Revision, Preferences: saved.Settings.Preferences, Credentials: &CredentialsWrite{Email: new(" NEW@example.com "), NewPassword: testutil.AdminPassword}})
	if err != nil || same.ReauthenticationRequired {
		t.Fatal("unchanged credentials revoked session", err)
	}
	if _, err := authSessions.CurrentSession(t.Context(), fresh.Token); err != nil {
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
	raw, err := Load(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	raw.Auth.Email = testutil.AdminEmail
	backup.PanelSettings, err = encodeSettings(raw, backup.PanelSettings, true)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := app.database.ConfigurationFile(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RestorePanelBackup(t.Context(), PanelRestoreRequest{Backup: backup, SettingsRevision: same.Settings.Revision, ConfigurationRevision: configuration.Revision})
	if err != nil || !result.ReauthenticationRequired {
		t.Fatal("restore credentials", err)
	}
	if _, err := authSessions.CurrentSession(t.Context(), fresh.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("restore did not revoke", err)
	}
}
