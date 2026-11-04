// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"sync"
)

// Reasons a session ends from the panel's side.
var (
	errSessionReplaced = errors.New("a newer session for this node replaced this one")
	errNodeRemoved     = errors.New("the node was removed or enrolled again")
	errPanelStopping   = errors.New("the panel is stopping")
)

// session is one authenticated node session.
type session struct {
	cancel context.CancelCauseFunc
}

// sessions holds the current session of each connected node.
type sessions struct {
	mu     sync.Mutex
	byNode map[string]*session
}

// replace makes current the node's session and ends the previous one.
func (registry *sessions) replace(nodeID string, current *session) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if previous := registry.byNode[nodeID]; previous != nil {
		previous.cancel(errSessionReplaced)
	}
	registry.byNode[nodeID] = current
}

// remove forgets current if it is still the node's session.
func (registry *sessions) remove(nodeID string, current *session) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.byNode[nodeID] == current {
		delete(registry.byNode, nodeID)
	}
}

// close ends the node's session, if any, with cause.
func (registry *sessions) close(nodeID string, cause error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if current := registry.byNode[nodeID]; current != nil {
		current.cancel(cause)
		delete(registry.byNode, nodeID)
	}
}

func (registry *sessions) closeAll(cause error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for nodeID, current := range registry.byNode {
		current.cancel(cause)
		delete(registry.byNode, nodeID)
	}
}
