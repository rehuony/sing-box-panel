// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/settingsfile"
)

// credentialMemo holds the credentials parsed from the settings document last
// read for them. It is keyed by the complete document, so it never answers
// for other bytes.
type credentialMemo struct {
	sync.Mutex
	email, passwordHash string
	document            []byte
}

// CurrentCredentials returns the saved administrator credentials without the
// settings lock, so session checks wait neither for each other nor for a
// writer. The file is read on every call and parsed again only when its bytes
// differ from the memo's.
//
// A change by any process, the command line or a manual edit included, takes
// effect for the next call: comparing whole documents misses no change, unlike
// file timestamps, whose resolution can hide an in-place edit. Every writer
// publishes a complete document by atomic replacement, so a lock-free read sees
// one whole document. Read refuses while a panel save's recovery journal
// exists, because that save can still roll back; only then is the lock taken,
// to recover first as every locked reader does.
func (app *Service) CurrentCredentials(ctx context.Context) (email, passwordHash string, err error) {
	if app.settingsPath == "" {
		return app.settings.Auth.Email, app.settings.Auth.PasswordHash, nil
	}
	raw, err := Read(app.settingsPath)
	if errors.Is(err, ErrPending) {
		err = app.WithCredentials(ctx, func(savedEmail, savedHash string) error {
			email, passwordHash = savedEmail, savedHash
			return nil
		})
		return email, passwordHash, err
	}
	if err != nil {
		return "", "", err
	}
	memo := &app.credentials
	memo.Lock()
	hit := memo.document != nil && bytes.Equal(memo.document, raw)
	email, passwordHash = memo.email, memo.passwordHash
	memo.Unlock()
	if hit {
		return email, passwordHash, nil
	}
	value, err := Parse(app.settingsPath, raw)
	if err != nil {
		return "", "", err
	}
	memo.Lock()
	memo.document, memo.email, memo.passwordHash = raw, value.Auth.Email, value.Auth.PasswordHash
	memo.Unlock()
	return value.Auth.Email, value.Auth.PasswordHash, nil
}

// WithCredentials runs action with the administrator credentials while holding
// the settings lock, after recovering an interrupted save, so a cooperating
// credential writer cannot interleave a stale snapshot with session work.
func (app *Service) WithCredentials(ctx context.Context, action func(email, passwordHash string) error) error {
	if app.settingsPath == "" {
		return action(app.settings.Auth.Email, app.settings.Auth.PasswordHash)
	}
	lock, err := settingsfile.Lock(ctx, app.settingsPath)
	if err != nil {
		return err
	}
	defer lock.Close()
	value, _, err := app.CurrentLocked(ctx)
	if err != nil {
		return err
	}
	return action(value.Auth.Email, value.Auth.PasswordHash)
}
