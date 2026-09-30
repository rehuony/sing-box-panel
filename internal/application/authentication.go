// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const SessionLifetime = 7 * 24 * time.Hour

var ErrInvalidCredentials = errors.New("email or password is incorrect")

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthSession struct {
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CSRFToken   string    `json:"csrfToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Token       string    `json:"-"`
}

// withAuthenticationSettings holds the shared settings lock until session reads
// or mutations finish. A cooperating credential writer cannot interleave a stale
// snapshot with the creation or reconciliation of a newer session.
func (app *Application) withAuthenticationSettings(ctx context.Context, action func(settings.Settings) error) error {
	if app.database == nil {
		return errors.New("authentication storage is unavailable")
	}
	if app.settingsPath == "" {
		return action(app.settings)
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return err
	}
	defer lock.Close()
	value, _, err := app.currentSettingsLocked(ctx)
	if err != nil {
		return err
	}
	return action(value)
}

func (app *Application) Login(ctx context.Context, input LoginInput, previous string) (result AuthSession, err error) {
	err = app.withAuthenticationSettings(ctx, func(value settings.Settings) error {
		email, emailErr := auth.NormalizeEmail(input.Email)
		matches, err := auth.VerifyPassword(ctx, input.Password, value.Auth.PasswordHash)
		if err != nil {
			return err
		}
		if !matches || emailErr != nil || email != value.Auth.Email {
			return ErrInvalidCredentials
		}
		credential := auth.Fingerprint(value.Auth.Email, value.Auth.PasswordHash)
		now := app.now()
		if err := app.database.ReconcileAuthSessions(ctx, credential, now); err != nil {
			return err
		}
		raw, err := app.sessionToken(32)
		if err != nil {
			return err
		}
		csrf, err := app.sessionToken(24)
		if err != nil {
			return err
		}
		expiresAt := now.Add(SessionLifetime).UTC().Truncate(time.Second)
		if err := app.database.CreateAuthSession(ctx, store.AuthSession{TokenHash: sha256.Sum256([]byte(raw)), CSRFToken: csrf, ExpiresAt: expiresAt, CredentialHash: credential}, sha256.Sum256([]byte(previous))); err != nil {
			return err
		}
		result = AuthSession{Email: value.Auth.Email, DisplayName: "Administrator", CSRFToken: csrf, ExpiresAt: expiresAt, Token: raw}
		return nil
	})
	return result, err
}

func (app *Application) CurrentSession(ctx context.Context, token string) (result AuthSession, err error) {
	if token == "" || len(token) > 128 {
		return result, store.ErrAuthSessionMissing
	}
	err = app.withAuthenticationSettings(ctx, func(value settings.Settings) error {
		credential := auth.Fingerprint(value.Auth.Email, value.Auth.PasswordHash)
		if err := app.database.ReconcileAuthSessions(ctx, credential, app.now()); err != nil {
			return err
		}
		session, err := app.database.AuthSession(ctx, sha256.Sum256([]byte(token)), credential, app.now())
		if err != nil {
			return err
		}
		result = AuthSession{Email: value.Auth.Email, DisplayName: "Administrator", CSRFToken: session.CSRFToken, ExpiresAt: session.ExpiresAt}
		return nil
	})
	return result, err
}

func (app *Application) Logout(ctx context.Context, token string) error {
	return app.database.DeleteAuthSession(ctx, sha256.Sum256([]byte(token)))
}

func (app *Application) sessionToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := app.random(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
