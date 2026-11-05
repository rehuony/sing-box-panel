// SPDX-License-Identifier: GPL-3.0-or-later
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/webauth"
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

// Credentials supplies the administrator credentials. WithCredentials holds the
// shared settings lock until action returns, so a cooperating credential writer
// cannot interleave a stale snapshot with the creation or reconciliation of a
// newer session.
type Credentials interface {
	WithCredentials(ctx context.Context, action func(email, passwordHash string) error) error
}

// Sessions issues, restores and revokes administrator login sessions.
type Sessions struct {
	database    *store.Store
	credentials Credentials
	now         func() time.Time
	random      func([]byte) (int, error)
}

func NewSessions(database *store.Store, credentials Credentials) *Sessions {
	return &Sessions{database: database, credentials: credentials, now: time.Now, random: rand.Read}
}

func (app *Sessions) withCredentials(ctx context.Context, action func(email, passwordHash string) error) error {
	if app.database == nil || app.credentials == nil {
		return errors.New("authentication storage is unavailable")
	}
	return app.credentials.WithCredentials(ctx, action)
}

func (app *Sessions) Login(ctx context.Context, input LoginInput, previous string) (result AuthSession, err error) {
	err = app.withCredentials(ctx, func(adminEmail, passwordHash string) error {
		email, emailErr := NormalizeEmail(input.Email)
		matches, err := webauth.VerifyPassword(ctx, input.Password, passwordHash)
		if err != nil {
			return err
		}
		if !matches || emailErr != nil || email != adminEmail {
			return ErrInvalidCredentials
		}
		credential := Fingerprint(adminEmail, passwordHash)
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
		result = AuthSession{Email: adminEmail, DisplayName: "Administrator", CSRFToken: csrf, ExpiresAt: expiresAt, Token: raw}
		return nil
	})
	return result, err
}

func (app *Sessions) CurrentSession(ctx context.Context, token string) (result AuthSession, err error) {
	if token == "" || len(token) > 128 {
		return result, store.ErrAuthSessionMissing
	}
	err = app.withCredentials(ctx, func(adminEmail, passwordHash string) error {
		credential := Fingerprint(adminEmail, passwordHash)
		if err := app.database.ReconcileAuthSessions(ctx, credential, app.now()); err != nil {
			return err
		}
		session, err := app.database.AuthSession(ctx, sha256.Sum256([]byte(token)), credential, app.now())
		if err != nil {
			return err
		}
		result = AuthSession{Email: adminEmail, DisplayName: "Administrator", CSRFToken: session.CSRFToken, ExpiresAt: session.ExpiresAt}
		return nil
	})
	return result, err
}

func (app *Sessions) Logout(ctx context.Context, token string) error {
	return app.database.DeleteAuthSession(ctx, sha256.Sum256([]byte(token)))
}

func (app *Sessions) sessionToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := app.random(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
