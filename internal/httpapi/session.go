// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"
)

const sessionLifetime = 12 * time.Hour

type session struct {
	csrf      string
	expiresAt time.Time
}

type sessions struct {
	mu     sync.Mutex
	values map[[32]byte]session
	now    func() time.Time
}

func newSessions() *sessions {
	return &sessions{values: make(map[[32]byte]session), now: time.Now}
}

func (manager *sessions) create() (raw, csrf string, expiresAt time.Time, err error) {
	raw, err = randomOpaqueToken(32)
	if err != nil {
		return "", "", time.Time{}, err
	}
	csrf, err = randomOpaqueToken(24)
	if err != nil {
		return "", "", time.Time{}, err
	}
	expiresAt = manager.now().Add(sessionLifetime)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.removeExpiredLocked()
	manager.values[sha256.Sum256([]byte(raw))] = session{csrf: csrf, expiresAt: expiresAt}
	return raw, csrf, expiresAt, nil
}

func (manager *sessions) find(raw string) (session, bool) {
	if raw == "" {
		return session{}, false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.removeExpiredLocked()
	value, ok := manager.values[sha256.Sum256([]byte(raw))]
	return value, ok
}

func (manager *sessions) delete(raw string) {
	manager.mu.Lock()
	delete(manager.values, sha256.Sum256([]byte(raw)))
	manager.mu.Unlock()
}

func (manager *sessions) removeExpiredLocked() {
	now := manager.now()
	for key, value := range manager.values {
		if !value.expiresAt.After(now) {
			delete(manager.values, key)
		}
	}
}

func randomOpaqueToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func constantTimeTokenEqual(left, right string) bool {
	leftHash, rightHash := sha256.Sum256([]byte(left)), sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}
