// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestHashPasswordCommand(t *testing.T) {
	password := "  twelve 密码 characters  "
	var stdout, stderr bytes.Buffer
	command := NewRootCommand(Dependencies{Stdin: strings.NewReader(password + "\n"), Stdout: &stdout, Stderr: &stderr})
	command.SetArgs([]string{"config", "hash-password", "--stdin"})
	if err := command.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimSpace(stdout.String())
	matches, err := auth.VerifyPassword(t.Context(), password, hash)
	if err != nil || !matches || strings.Contains(stdout.String()+stderr.String(), password) {
		t.Fatal("stdin password was altered or exposed", err)
	}
	for _, args := range [][]string{{"config", "hash-password"}, {"config", "hash-password", "plain-password"}, {"config", "hash-password", "--password", "plain-password"}} {
		stdout.Reset()
		stderr.Reset()
		command = NewRootCommand(Dependencies{Stdin: strings.NewReader(password), Stdout: &stdout, Stderr: &stderr})
		command.SetArgs(args)
		if err := command.ExecuteContext(t.Context()); err == nil || stdout.Len() != 0 {
			t.Fatal("accepted implicit stdin or plaintext argument")
		}
	}
}

func TestResetPasswordCommand(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			path := commandSettingsFixture(t)
			before, err := settings.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runPanelConfig(t, t.Context(), path, nil, "reset-password", "-o", format)
			if err != nil {
				t.Fatal(err)
			}
			after, err := settings.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			var password string
			if format == "text" {
				match := regexp.MustCompile(`(?m)^  New Password  ([^\r\n]+)$`).FindStringSubmatch(out)
				if len(match) != 2 {
					t.Fatal("missing password output")
				}
				password = match[1]
			} else {
				var result struct {
					Reset    bool   `json:"password_reset"`
					Path     string `json:"settings_path"`
					Email    string `json:"login_email"`
					Password string `json:"login_password"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil || !result.Reset || result.Path != path || result.Email != before.Auth.Email {
					t.Fatal("invalid reset result", err)
				}
				password = result.Password
			}
			matches, err := auth.VerifyPassword(t.Context(), password, after.Auth.PasswordHash)
			if err != nil || !matches || after.Auth.PasswordHash == before.Auth.PasswordHash {
				t.Fatal("password not rotated", err)
			}
			if strings.Count(out, password) != 1 || strings.Contains(out, after.Auth.PasswordHash) || strings.Contains(out, "\x1b[") {
				t.Fatal("invalid credential output")
			}
			if _, err := os.Stat(filepath.Join(before.DataDir, "panel.db")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("reset opened storage", err)
			}
			password = "  custom 密码 value  "
			out, err = runPanelConfig(t, t.Context(), path, strings.NewReader(password+"\r\n"), "reset-password", "--stdin", "-o", format)
			if err != nil || strings.Contains(out, password) || strings.Contains(out, "login_password") {
				t.Fatal("custom password exposed", err)
			}
			after, err = settings.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if matches, err := auth.VerifyPassword(t.Context(), password, after.Auth.PasswordHash); err != nil || !matches {
				t.Fatal("custom password altered", err)
			}
		})
	}
}

func TestResetPasswordChangesRunningAuthenticationWithoutRestart(t *testing.T) {
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
	if _, err := runPanelConfig(t, t.Context(), path, strings.NewReader(testutil.AdminPassword), "reset-password", "--stdin"); err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if _, err := app.CurrentSession(t.Context(), session.Token); !errors.Is(err, store.ErrAuthSessionMissing) {
			t.Fatal("old session survived reset", err)
		}
	}
	if _, err := runPanelConfig(t, t.Context(), path, strings.NewReader(testutil.ChangedPassword), "reset-password", "--stdin"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Login(t.Context(), application.LoginInput{Email: testutil.AdminEmail, Password: testutil.AdminPassword}, ""); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatal("old password accepted", err)
	}
	if _, err := app.Login(t.Context(), application.LoginInput{Email: testutil.AdminEmail, Password: testutil.ChangedPassword}, ""); err != nil {
		t.Fatal("new password not usable without restart", err)
	}
}

func TestResetPasswordRejectsArgumentsAndInvalidInput(t *testing.T) {
	path := commandSettingsFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"reset-password", "plain-password"}, {"reset-password", "--password", "plain-password"}, {"reset-password", "--stdin"}} {
		out, err := runPanelConfig(t, t.Context(), path, strings.NewReader("short"), args...)
		after, readErr := os.ReadFile(path)
		if err == nil || out != "" || readErr != nil || !bytes.Equal(before, after) {
			t.Fatal("invalid reset modified settings or emitted credentials", err)
		}
	}
}
