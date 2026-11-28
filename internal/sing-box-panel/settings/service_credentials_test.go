// SPDX-License-Identifier: GPL-3.0-or-later
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
	"github.com/rehuony/sing-box-panel/internal/webauth"
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
		// Password checks run outside the settings lock, at most two at once.
		if !errors.Is(err, auth.ErrInvalidCredentials) && !errors.Is(err, webauth.ErrBusy) {
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
}

func TestSessionChecksRunWhileTheSettingsLockIsHeld(t *testing.T) {
	app := panelFileApp(t)
	authSessions := auth.NewSessions(app.database, app)
	session, err := authSessions.Login(t.Context(), auth.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, "")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := settingsfile.Lock(t.Context(), app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	failures := make(chan error, 8)
	var checks sync.WaitGroup
	for range 8 {
		checks.Go(func() {
			if _, err := authSessions.CurrentSession(ctx, session.Token); err != nil {
				failures <- err
			}
		})
	}
	checks.Wait()
	close(failures)
	for err := range failures {
		t.Fatal("session check waited for the settings lock", err)
	}
}

func TestSessionChecksFollowEveryCredentialWriter(t *testing.T) {
	app := panelFileApp(t)
	authSessions := auth.NewSessions(app.database, app)
	login := func(email string) auth.AuthSession {
		t.Helper()
		session, err := authSessions.Login(t.Context(), auth.LoginInput{Email: email, Password: testutil.AdminPassword}, "")
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	check := func(session auth.AuthSession) error {
		_, err := authSessions.CurrentSession(t.Context(), session.Token)
		return err
	}
	session := login(testutil.AdminEmail)

	// An interrupted save that never committed rolls back before its file counts.
	before, _ := Read(app.settingsPath)
	value, _ := Load(app.settingsPath)
	value.Auth.Email = "pending@example.com"
	after, _ := json.Marshal(value)
	journal, _ := json.Marshal(settingsJournal{ID: "interrupted", Before: before, After: after})
	if err := WriteAtomic(app.settingsPath+".pending", journal); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceLocked(app.settingsPath, after); err != nil {
		t.Fatal(err)
	}
	if err := check(session); err != nil {
		t.Fatal("uncommitted credentials applied", err)
	}

	// The command line replaces the file atomically.
	if err := SetField(t.Context(), app.settingsPath, "auth.email", "other@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := check(session); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("session survived a command-line credential change", err)
	}
	session = login("other@example.com")

	// A manual edit in place keeps the file's size and modification time.
	info, err := os.Stat(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(app.settingsPath)
	edited := bytes.Replace(raw, []byte("other@example.com"), []byte("otter@example.com"), 1)
	if err := os.WriteFile(app.settingsPath, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(app.settingsPath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := check(session); !errors.Is(err, store.ErrAuthSessionMissing) {
		t.Fatal("session survived a manual credential edit", err)
	}
}
