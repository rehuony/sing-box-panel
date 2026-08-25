// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net"
	"strings"
	"sync"
	"time"
)

const (
	loginFailureLimit  = 5
	loginFailureWindow = time.Minute
	maxLoginClients    = 4096
)

type loginAttempt struct {
	failures  int
	expiresAt time.Time
}

// loginLimiter is deliberately local to the process. It bounds online guessing
// without trusting proxy-supplied headers or creating attacker-controlled rows
// in the database.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginAttempt
	now     func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]loginAttempt), now: time.Now}
}

func (limiter *loginLimiter) allow(client string) (bool, time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	limiter.removeExpiredLocked(now)
	attempt, ok := limiter.entries[client]
	if !ok || attempt.failures < loginFailureLimit {
		return true, 0
	}
	return false, attempt.expiresAt.Sub(now)
}

func (limiter *loginLimiter) failed(client string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	limiter.removeExpiredLocked(now)
	attempt, ok := limiter.entries[client]
	if !ok {
		limiter.makeRoomLocked()
		attempt.expiresAt = now.Add(loginFailureWindow)
	}
	attempt.failures++
	limiter.entries[client] = attempt
}

func (limiter *loginLimiter) succeeded(client string) {
	limiter.mu.Lock()
	delete(limiter.entries, client)
	limiter.mu.Unlock()
}

func (limiter *loginLimiter) removeExpiredLocked(now time.Time) {
	for client, attempt := range limiter.entries {
		if !attempt.expiresAt.After(now) {
			delete(limiter.entries, client)
		}
	}
}

func (limiter *loginLimiter) makeRoomLocked() {
	if len(limiter.entries) < maxLoginClients {
		return
	}
	var oldestClient string
	var oldestExpiry time.Time
	for client, attempt := range limiter.entries {
		if oldestClient == "" || attempt.expiresAt.Before(oldestExpiry) {
			oldestClient, oldestExpiry = client, attempt.expiresAt
		}
	}
	delete(limiter.entries, oldestClient)
}

func loginClient(remoteAddress string) string {
	remoteAddress = strings.TrimSpace(remoteAddress)
	if host, _, err := net.SplitHostPort(remoteAddress); err == nil && host != "" {
		return host
	}
	if remoteAddress == "" {
		return "unknown"
	}
	return remoteAddress
}
