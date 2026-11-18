// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

// Reasons a session ends from the panel's side.
var (
	errSessionReplaced = errors.New("a newer session for this node replaced this one")
	errNodeRemoved     = errors.New("the node was removed or enrolled again")
	errPanelStopping   = errors.New("the panel is stopping")
)

// session is one authenticated node session.
type session struct {
	nodeID     string
	instanceID string
	sessionID  string
	// protocolMinor and buildVersion are what the node's Hello named.
	protocolMinor uint32
	buildVersion  string
	// lastUpdate is the update attempt the panel last recorded for the
	// node; the session loop alone uses it.
	lastUpdate *nodeapi.UpdateAttempt
	// ready closes when the node's first state report is recorded; only
	// then does the session accept commands.
	ready     chan struct{}
	readyOnce sync.Once
	// telemetryMu orders Welcome and the session's metrics interval
	// changes; metricsInterval is the interval the node was last told,
	// starting with the baseline Welcome names.
	telemetryMu     sync.Mutex
	welcomed        bool
	metricsInterval time.Duration

	cancel context.CancelCauseFunc
	// done closes when the session ends.
	done <-chan struct{}
	// send delivers one frame to the node; frames are sent one at a time.
	send func(*nodeapi.ConnectResponse) error

	mu      sync.Mutex
	pending map[string]chan *nodeapi.Result
}

func newSession(ctx context.Context, cancel context.CancelCauseFunc, stream nodeapi.NodeService_ConnectServer) *session {
	var sendMu sync.Mutex
	return &session{
		ready: make(chan struct{}), metricsInterval: MetricsInterval,
		cancel: cancel, done: ctx.Done(), pending: map[string]chan *nodeapi.Result{},
		send: func(frame *nodeapi.ConnectResponse) error {
			sendMu.Lock()
			defer sendMu.Unlock()
			return stream.Send(frame)
		},
	}
}

// open lets the session accept commands.
func (current *session) open() {
	current.readyOnce.Do(func() { close(current.ready) })
}

// isReady reports whether the session accepts commands.
func (current *session) isReady() bool {
	select {
	case <-current.ready:
		return true
	default:
		return false
	}
}

// expect registers a command whose result deliver hands over.
func (current *session) expect(requestID string) chan *nodeapi.Result {
	current.mu.Lock()
	defer current.mu.Unlock()
	result := make(chan *nodeapi.Result, 1)
	current.pending[requestID] = result
	return result
}

func (current *session) forget(requestID string) {
	current.mu.Lock()
	defer current.mu.Unlock()
	delete(current.pending, requestID)
}

// deliver hands a node's result to the command waiting for it, if any.
func (current *session) deliver(result *nodeapi.Result) {
	current.mu.Lock()
	defer current.mu.Unlock()
	if waiting := current.pending[result.GetRequestId()]; waiting != nil {
		waiting <- result
		delete(current.pending, result.GetRequestId())
	}
}

// get returns the node's current session, if any.
func (registry *sessions) get(nodeID string) *session {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.byNode[nodeID]
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
