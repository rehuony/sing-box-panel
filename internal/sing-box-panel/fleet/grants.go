// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// grantSize is the number of random bytes in an artifact grant.
const grantSize = 32

// grants holds the short-lived grants that let a node fetch one cached
// artifact while a command installs it. Only digests of grants are kept.
type grants struct {
	mu      sync.Mutex
	byToken map[[sha256.Size]byte]grant
}

type grant struct {
	nodeID     string
	artifactID string
	expiresAt  time.Time
}

// issue returns a grant for nodeID to fetch artifactID until expiresAt.
func (registry *grants) issue(random func([]byte) (int, error), nodeID, artifactID string, expiresAt time.Time) (string, error) {
	raw := make([]byte, grantSize)
	if n, err := random(raw); err != nil || n != len(raw) {
		return "", errors.Join(errors.New("generate artifact grant"), err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.byToken == nil {
		registry.byToken = map[[sha256.Size]byte]grant{}
	}
	registry.byToken[sha256.Sum256([]byte(token))] = grant{nodeID: nodeID, artifactID: artifactID, expiresAt: expiresAt}
	return token, nil
}

func (registry *grants) revoke(token string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	delete(registry.byToken, sha256.Sum256([]byte(token)))
}

// check reports whether token grants artifactID at now, and to which node.
func (registry *grants) check(token, artifactID string, now time.Time) (string, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key := sha256.Sum256([]byte(token))
	current, ok := registry.byToken[key]
	if !ok || current.artifactID != artifactID {
		return "", false
	}
	if !now.Before(current.expiresAt) {
		delete(registry.byToken, key)
		return "", false
	}
	return current.nodeID, true
}
