// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	mathrand "math/rand/v2"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
)

// ConnectionState is where a node's link with its panel stands.
type ConnectionState string

const (
	// ConnectionUnenrolled: the node has no panel.
	ConnectionUnenrolled ConnectionState = "unenrolled"
	// ConnectionConnecting: a session is being opened.
	ConnectionConnecting ConnectionState = "connecting"
	// ConnectionConnected: a session is open and serving commands.
	ConnectionConnected ConnectionState = "connected"
	// ConnectionDisconnected: the last session ended; the node retries at
	// RetryAt.
	ConnectionDisconnected ConnectionState = "disconnected"
)

// maximumErrorBytes bounds the reason a status carries.
const maximumErrorBytes = 512

// Status is a snapshot of a node's link with its panel.
type Status struct {
	// Enrollment is the panel the node is enrolled with; nil when unenrolled.
	Enrollment *EnrollmentSummary
	State      ConnectionState
	// Since is when the link entered State.
	Since time.Time
	// LastError is why the latest session ended or could not open; a
	// session that opens clears it.
	LastError string
	// RetryAt is when a disconnected node opens its next session.
	RetryAt *time.Time
}

// EnrollmentSummary is an enrollment without the node's key.
type EnrollmentSummary struct {
	Endpoint string
	Path     string
	PanelID  string
	NodeID   string
	// Pinned reports that the panel's certificate is pinned instead of
	// verified by the system's certificate authorities.
	Pinned bool
}

// Link is a node's enrollment with its panel and the state of its session.
// While the node runs, the local console may replace or remove the
// enrollment; the node then ends its session and starts over.
type Link struct {
	dataDir string
	build   buildinfo.Info
	now     func() time.Time

	// changing serializes enrollment changes.
	changing sync.Mutex

	mu         sync.Mutex
	enrollment *settings.Enrollment
	// generation counts enrollment changes, so a session of a replaced
	// enrollment cannot overwrite the status of the current one.
	generation uint64
	// changed is closed when the enrollment changes.
	changed chan struct{}
	status  Status
}

// OpenLink reads the enrollment in dataDir; a node that is not enrolled has
// a link without one.
func OpenLink(dataDir string, build buildinfo.Info) (*Link, error) {
	link := &Link{dataDir: dataDir, build: build, now: time.Now, changed: make(chan struct{})}
	enrollment, err := settings.LoadEnrollment(dataDir)
	switch {
	case errors.Is(err, settings.ErrNotEnrolled):
		link.setLocked(nil)
	case err != nil:
		return nil, err
	default:
		link.setLocked(&enrollment)
	}
	return link, nil
}

// Enrolled reports whether the node has a panel.
func (link *Link) Enrolled() bool {
	link.mu.Lock()
	defer link.mu.Unlock()
	return link.enrollment != nil
}

// Status returns the link's current state.
func (link *Link) Status() Status {
	link.mu.Lock()
	defer link.mu.Unlock()
	status := link.status
	if status.Enrollment != nil {
		enrollment := *status.Enrollment
		status.Enrollment = &enrollment
	}
	if status.RetryAt != nil {
		retryAt := *status.RetryAt
		status.RetryAt = &retryAt
	}
	return status
}

// Enroll exchanges an enrollment code for a new identity, replacing the
// node's enrollment, and ends the current session so the node connects to
// the panel the code names.
func (link *Link) Enroll(ctx context.Context, code string) (Status, error) {
	link.changing.Lock()
	defer link.changing.Unlock()
	enrollment, err := Enroll(ctx, link.dataDir, code, link.build)
	if err != nil {
		return link.Status(), err
	}
	link.replace(&enrollment)
	return link.Status(), nil
}

// Unenroll removes the node's enrollment and ends its session. The panel
// keeps the node, shown offline, until an administrator removes it; the
// node's runtime keeps its applied configuration.
func (link *Link) Unenroll() (Status, error) {
	link.changing.Lock()
	defer link.changing.Unlock()
	if err := settings.RemoveEnrollment(link.dataDir); err != nil {
		return link.Status(), err
	}
	link.replace(nil)
	return link.Status(), nil
}

func (link *Link) replace(enrollment *settings.Enrollment) {
	link.mu.Lock()
	defer link.mu.Unlock()
	link.setLocked(enrollment)
	link.generation++
	close(link.changed)
	link.changed = make(chan struct{})
}

func (link *Link) setLocked(enrollment *settings.Enrollment) {
	link.enrollment = enrollment
	link.status = Status{State: ConnectionUnenrolled, Since: link.now()}
	if enrollment != nil {
		link.status = Status{
			Enrollment: &EnrollmentSummary{
				Endpoint: enrollment.Endpoint, Path: enrollment.Path, PanelID: enrollment.PanelID, NodeID: enrollment.NodeID,
				Pinned: enrollment.Pin != "",
			},
			State: ConnectionConnecting, Since: link.now(),
		}
	}
}

// current returns the enrollment, its generation and a channel closed when
// it changes.
func (link *Link) current() (*settings.Enrollment, uint64, <-chan struct{}) {
	link.mu.Lock()
	defer link.mu.Unlock()
	return link.enrollment, link.generation, link.changed
}

// report records the state of a session of the enrollment of generation.
func (link *Link) report(generation uint64, state ConnectionState, reason string, retryAt *time.Time) {
	link.mu.Lock()
	defer link.mu.Unlock()
	if generation != link.generation || link.enrollment == nil {
		return
	}
	link.status.State, link.status.Since, link.status.RetryAt = state, link.now(), retryAt
	switch state {
	case ConnectionConnected:
		link.status.LastError = ""
	case ConnectionDisconnected:
		if len(reason) > maximumErrorBytes {
			reason = reason[:maximumErrorBytes]
		}
		link.status.LastError = reason
	}
}

// session holds one session of enrollment until it ends or ctx ends, and
// calls connected once the session serves commands.
type session func(ctx context.Context, enrollment settings.Enrollment, connected func()) error

// supervise holds sessions of the current enrollment until ctx ends,
// reconnecting with backoff and starting over whenever the enrollment
// changes. Without an enrollment it waits for one.
func (link *Link) supervise(ctx context.Context, hold session, logf func(string, ...any)) {
	for {
		enrollment, generation, changed := link.current()
		if enrollment == nil {
			select {
			case <-ctx.Done():
				return
			case <-changed:
				continue
			}
		}
		attempts, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-changed:
				cancel()
			case <-attempts.Done():
			}
		}()
		link.reconnect(attempts, generation, *enrollment, hold, logf)
		cancel()
		if ctx.Err() != nil {
			return
		}
	}
}

// reconnect holds sessions of one enrollment until ctx ends.
func (link *Link) reconnect(ctx context.Context, generation uint64, enrollment settings.Enrollment, hold session, logf func(string, ...any)) {
	backoff := minimumBackoff
	for {
		link.report(generation, ConnectionConnecting, "", nil)
		started := time.Now()
		err := hold(ctx, enrollment, func() { link.report(generation, ConnectionConnected, "", nil) })
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > maximumBackoff {
			backoff = minimumBackoff
		}
		wait := backoff/2 + mathrand.N(backoff/2+1)
		retryAt := link.now().Add(wait)
		link.report(generation, ConnectionDisconnected, describe(err), &retryAt)
		logf("session with panel %s ended: %s; reconnecting in %s", enrollment.Endpoint, describe(err), wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		backoff = min(2*backoff, maximumBackoff)
	}
}

// watch replaces the enrollment with the one in the data directory whenever
// that changes, every interval until ctx ends, as the console's enrollment
// does: an operator who enrolls the node with sing-box-node enroll while it
// runs switches it to the new panel. A missing or unreadable file keeps the
// current enrollment.
func (link *Link) watch(ctx context.Context, interval time.Duration, logf func(string, ...any)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			link.reload(logf)
		}
	}
}

// reload switches to the enrollment saved in the data directory if it
// differs from the current one.
func (link *Link) reload(logf func(string, ...any)) bool {
	link.changing.Lock()
	defer link.changing.Unlock()
	saved, err := settings.LoadEnrollment(link.dataDir)
	if err != nil {
		return false
	}
	current, _, _ := link.current()
	if current != nil && sameEnrollment(*current, saved) {
		return false
	}
	link.replace(&saved)
	logf("the saved enrollment changed; switching to node %s of the panel at %s%s", saved.NodeID, saved.Endpoint, saved.Path)
	return true
}

func sameEnrollment(a, b settings.Enrollment) bool {
	return a.Endpoint == b.Endpoint && a.Path == b.Path && a.Pin == b.Pin && a.PanelID == b.PanelID && a.NodeID == b.NodeID && a.Key.Equal(b.Key)
}
