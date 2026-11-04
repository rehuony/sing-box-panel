// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	loginAttemptLimit   = 5
	loginAttemptWindow  = time.Minute
	loginGlobalLimit    = 30
	maxConcurrentLogins = 2
	maxLoginClients     = 4096
)

type loginAttempt struct {
	attempts  int
	expiresAt time.Time
}

// loginLimiter bounds work before reading a body, waiting for settings, or
// hashing a password. Fingerprints are untrusted supplementary signals, never
// replacements for the peer and process-wide budgets. State is memory-only.
type loginLimiter struct {
	mu          sync.Mutex
	entries     map[string]loginAttempt
	global      loginAttempt
	inFlight    int
	nextCleanup time.Time
	now         func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]loginAttempt), now: time.Now}
}

// begin atomically consumes the peer/global budgets and reserves a work slot.
// Rejected traffic from an exhausted peer cannot consume other peers' budget.
func (limiter *loginLimiter) begin(client string) (bool, time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	if !limiter.global.expiresAt.After(now) {
		limiter.global = loginAttempt{expiresAt: now.Add(loginAttemptWindow)}
	}
	if limiter.global.attempts >= loginGlobalLimit {
		return false, limiter.global.expiresAt.Sub(now)
	}
	if limiter.inFlight >= maxConcurrentLogins {
		return false, time.Second
	}
	if allowed, retry := limiter.consumeLocked("peer:"+client, now); !allowed {
		return false, retry
	}
	limiter.global.attempts++
	limiter.inFlight++
	return true, 0
}

func (limiter *loginLimiter) finish() {
	limiter.mu.Lock()
	limiter.inFlight--
	limiter.mu.Unlock()
}

func (limiter *loginLimiter) fingerprint(value string) (bool, time.Duration) {
	if value == "" {
		return true, 0
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	return limiter.consumeLocked("fingerprint:"+value, limiter.now())
}

// consumeLocked never evicts a live entry to admit an attacker-controlled key.
func (limiter *loginLimiter) consumeLocked(key string, now time.Time) (bool, time.Duration) {
	if !limiter.nextCleanup.After(now) {
		for key, attempt := range limiter.entries {
			if !attempt.expiresAt.After(now) {
				delete(limiter.entries, key)
			}
		}
		limiter.nextCleanup = now.Add(loginAttemptWindow)
	}
	attempt, exists := limiter.entries[key]
	if !exists && len(limiter.entries) >= maxLoginClients {
		return false, limiter.nextCleanup.Sub(now)
	}
	if !attempt.expiresAt.After(now) {
		attempt = loginAttempt{expiresAt: now.Add(loginAttemptWindow)}
	}
	if attempt.attempts >= loginAttemptLimit {
		return false, attempt.expiresAt.Sub(now)
	}
	attempt.attempts++
	limiter.entries[key] = attempt
	return true, 0
}

func loginClient(remoteAddress string) string {
	host := strings.TrimSpace(remoteAddress)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap().WithZone("")
		if address.Is6() {
			// Rotating addresses within a delegated IPv6 network is not a new peer.
			return netip.PrefixFrom(address, 64).Masked().String()
		}
		return address.String()
	}
	return "unknown"
}
