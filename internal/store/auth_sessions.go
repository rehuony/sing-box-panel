// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const authenticationSchema = `
CREATE TABLE auth_sessions (
 token_hash BLOB PRIMARY KEY CHECK(length(token_hash) = 32),
 csrf_token TEXT NOT NULL,
 expires_at INTEGER NOT NULL,
 credential_hash BLOB NOT NULL CHECK(length(credential_hash) = 32)
) STRICT;
CREATE INDEX auth_sessions_expiry ON auth_sessions(expires_at);
`

type AuthSession struct {
	TokenHash      [32]byte
	CSRFToken      string
	ExpiresAt      time.Time
	CredentialHash [32]byte
}

var ErrAuthSessionMissing = errors.New("session is missing or expired")

// ReconcileAuthSessions removes revoked credentials as well as expired sessions.
// Callers serialize the settings snapshot with credential writes before calling.
func (s *Store) ReconcileAuthSessions(ctx context.Context, credential [32]byte, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE credential_hash <> ? OR expires_at <= ?`, credential[:], now.Unix())
	return err
}

func (s *Store) CreateAuthSession(ctx context.Context, value AuthSession, previous [32]byte) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, previous[:]); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO auth_sessions(token_hash, csrf_token, expires_at, credential_hash) VALUES(?,?,?,?)`, value.TokenHash[:], value.CSRFToken, value.ExpiresAt.Unix(), value.CredentialHash[:])
		return err
	})
}

func (s *Store) AuthSession(ctx context.Context, token, credential [32]byte, now time.Time) (AuthSession, error) {
	value := AuthSession{TokenHash: token, CredentialHash: credential}
	var expiry int64
	err := s.db.QueryRowContext(ctx, `SELECT csrf_token, expires_at FROM auth_sessions WHERE token_hash = ? AND credential_hash = ? AND expires_at > ?`, token[:], credential[:], now.Unix()).Scan(&value.CSRFToken, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, ErrAuthSessionMissing
	}
	value.ExpiresAt = time.Unix(expiry, 0).UTC()
	return value, err
}

func (s *Store) DeleteAuthSession(ctx context.Context, token [32]byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, token[:])
	return err
}
