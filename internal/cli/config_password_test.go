// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestSetPasswordChangesRunningAuthenticationWithoutRestart(t *testing.T) {
	path := commandSettingsFixture(t)
	app, err := application.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	var sessions []application.AuthSession
	for range 2 {
		session, err := app.Login(t.Context(), application.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, "")
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, session)
	}
	// Supplying the same plaintext still resets sessions by creating a fresh hash.
	if _, err := runPanelConfig(t, t.Context(), path, strings.NewReader(testutil.AdminPassword), "set", "auth.password", "--stdin"); err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if _, err := app.CurrentSession(t.Context(), session.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
			t.Fatal("old session survived reset", err)
		}
	}
	if _, err := runPanelConfig(t, t.Context(), path, strings.NewReader(testutil.ChangedPassword), "set", "auth.password", "--stdin"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Login(t.Context(), application.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, ""); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatal("old password accepted", err)
	}
	if _, err := app.Login(t.Context(), application.LoginInput{Email: testutil.AdminEmail, Password: testutil.ChangedPassword}, ""); err != nil {
		t.Fatal("new password not usable without restart", err)
	}
}

func TestSetPasswordFieldHashesWithoutExposingValue(t *testing.T) {
	password := "  custom 密码 value  "
	for _, format := range []string{"text", "json", "jsonl"} {
		for _, input := range []string{"argument", "stdin"} {
			t.Run(format+input, func(t *testing.T) {
				path := commandSettingsFixture(t)
				args := []string{"set", "auth.password", password, "-o", format}
				if input == "stdin" {
					args = []string{"set", "auth.password", "--stdin", "-o", format}
				}
				out, err := runPanelConfig(t, t.Context(), path, strings.NewReader(password+"\r\n"), args...)
				if err != nil {
					t.Fatal(err)
				}
				value, err := settings.Load(path)
				if err != nil {
					t.Fatal(err)
				}
				if matches, err := auth.VerifyPassword(t.Context(), password, value.Auth.PasswordHash); err != nil || !matches {
					t.Fatal("password not hashed using the backend authentication contract", err)
				}
				data, err := os.ReadFile(path)
				if err != nil || bytes.Contains(data, []byte(password)) || bytes.Contains(data, []byte(`"password":`)) ||
					strings.Contains(out, password) || strings.Contains(out, value.Auth.PasswordHash) {
					t.Fatal("plaintext password persisted or credential echoed", err)
				}
			})
		}
	}
}
