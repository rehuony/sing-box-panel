// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

const (
	sessionLifetime = 12 * time.Hour
	// maximumSessions bounds the console's sessions; a new one replaces the
	// one that expires first.
	maximumSessions = 32
)

// session is one console login. Sessions live in memory, so restarting the
// node signs every browser out.
type session struct {
	csrf      string
	expiresAt time.Time
	// credential is the password the session logged in with.
	credential [32]byte
}

type sessions struct {
	mu      sync.Mutex
	byToken map[[32]byte]session
}

func newSessions() *sessions {
	return &sessions{byToken: map[[32]byte]session{}}
}

// create starts a session for credential and ends the previous one of the
// same browser.
func (sessions *sessions) create(credential [32]byte, now time.Time, previous string) (string, session, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", session{}, err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return "", session{}, err
	}
	created := session{csrf: csrf, expiresAt: now.Add(sessionLifetime).UTC().Truncate(time.Second), credential: credential}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	delete(sessions.byToken, sha256.Sum256([]byte(previous)))
	var oldest [32]byte
	for key, value := range sessions.byToken {
		if !value.expiresAt.After(now) {
			delete(sessions.byToken, key)
		} else if existing, ok := sessions.byToken[oldest]; !ok || value.expiresAt.Before(existing.expiresAt) {
			oldest = key
		}
	}
	if len(sessions.byToken) >= maximumSessions {
		delete(sessions.byToken, oldest)
	}
	sessions.byToken[sha256.Sum256([]byte(token))] = created
	return token, created, nil
}

// lookup returns the live session of token, which must have logged in with
// credential.
func (sessions *sessions) lookup(token string, credential [32]byte, now time.Time) (session, bool) {
	if token == "" || len(token) > 128 {
		return session{}, false
	}
	key := sha256.Sum256([]byte(token))
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	found, ok := sessions.byToken[key]
	if !ok || !found.expiresAt.After(now) || found.credential != credential {
		delete(sessions.byToken, key)
		return session{}, false
	}
	return found, true
}

func (sessions *sessions) remove(token string) {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	delete(sessions.byToken, sha256.Sum256([]byte(token)))
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
