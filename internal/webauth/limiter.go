// SPDX-License-Identifier: GPL-3.0-or-later

package webauth

import (
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	// LoginAttemptLimit is how many logins one peer or fingerprint may try
	// within LoginAttemptWindow.
	LoginAttemptLimit  = 5
	LoginAttemptWindow = time.Minute
	// MaxConcurrentLogins bounds the logins that verify a password at once.
	MaxConcurrentLogins = 2
	loginGlobalLimit    = 30
	maxLoginClients     = 4096
)

type loginAttempt struct {
	attempts  int
	expiresAt time.Time
}

// LoginLimiter bounds work before reading a body, waiting for settings, or
// hashing a password. Fingerprints are untrusted supplementary signals, never
// replacements for the peer and process-wide budgets. State is memory-only.
type LoginLimiter struct {
	mu          sync.Mutex
	entries     map[string]loginAttempt
	global      loginAttempt
	inFlight    int
	nextCleanup time.Time
	now         func() time.Time
}

// NewLoginLimiter returns a limiter that reads the time from now.
func NewLoginLimiter(now func() time.Time) *LoginLimiter {
	return &LoginLimiter{entries: make(map[string]loginAttempt), now: now}
}

// Begin atomically consumes the peer and global budgets and reserves a work
// slot, which Finish releases. Rejected traffic from an exhausted peer
// cannot consume other peers' budget.
func (limiter *LoginLimiter) Begin(client string) (bool, time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	if !limiter.global.expiresAt.After(now) {
		limiter.global = loginAttempt{expiresAt: now.Add(LoginAttemptWindow)}
	}
	if limiter.global.attempts >= loginGlobalLimit {
		return false, limiter.global.expiresAt.Sub(now)
	}
	if limiter.inFlight >= MaxConcurrentLogins {
		return false, time.Second
	}
	if allowed, retry := limiter.consumeLocked("peer:"+client, now); !allowed {
		return false, retry
	}
	limiter.global.attempts++
	limiter.inFlight++
	return true, 0
}

func (limiter *LoginLimiter) Finish() {
	limiter.mu.Lock()
	limiter.inFlight--
	limiter.mu.Unlock()
}

// Fingerprint consumes the budget of a client-supplied fingerprint; an empty
// fingerprint has none.
func (limiter *LoginLimiter) Fingerprint(value string) (bool, time.Duration) {
	if value == "" {
		return true, 0
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	return limiter.consumeLocked("fingerprint:"+value, limiter.now())
}

// consumeLocked never evicts a live entry to admit an attacker-controlled key.
func (limiter *LoginLimiter) consumeLocked(key string, now time.Time) (bool, time.Duration) {
	if !limiter.nextCleanup.After(now) {
		for key, attempt := range limiter.entries {
			if !attempt.expiresAt.After(now) {
				delete(limiter.entries, key)
			}
		}
		limiter.nextCleanup = now.Add(LoginAttemptWindow)
	}
	attempt, exists := limiter.entries[key]
	if !exists && len(limiter.entries) >= maxLoginClients {
		return false, limiter.nextCleanup.Sub(now)
	}
	if !attempt.expiresAt.After(now) {
		attempt = loginAttempt{expiresAt: now.Add(LoginAttemptWindow)}
	}
	if attempt.attempts >= LoginAttemptLimit {
		return false, attempt.expiresAt.Sub(now)
	}
	attempt.attempts++
	limiter.entries[key] = attempt
	return true, 0
}

// LoginClient is the peer a login is charged to: its address, or its /64
// network for IPv6. Forwarded headers are never trusted.
func LoginClient(remoteAddress string) string {
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
