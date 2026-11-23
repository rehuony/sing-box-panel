// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// maximumSnapshotFrameBytes bounds a frame of one node's streams.
const maximumSnapshotFrameBytes = 1 << 20

// eventStream writes server-sent events to one response. Each write gets its
// own deadline of ten seconds, never past the stream's deadline when it has
// one, and carries at most maximumFrameBytes.
type eventStream struct {
	ctx               context.Context
	w                 http.ResponseWriter
	controller        *http.ResponseController
	deadline          time.Time
	maximumFrameBytes int
	started           bool
}

// newEventStream reports false when the transport cannot stream.
func newEventStream(ctx context.Context, w http.ResponseWriter, deadline time.Time, maximumFrameBytes int) (*eventStream, bool) {
	if _, ok := w.(http.Flusher); !ok {
		return nil, false
	}
	return &eventStream{ctx: ctx, w: w, controller: http.NewResponseController(w), deadline: deadline, maximumFrameBytes: maximumFrameBytes}, true
}

// open sends the stream's headers.
func (stream *eventStream) open() error {
	return stream.write("")
}

// event sends one event named name whose data is data.
func (stream *eventStream) event(name string, data []byte) error {
	return stream.write(fmt.Sprintf("event: %s\ndata: %s\n\n", name, data))
}

// eventWithID sends one event that a reconnecting client resumes after.
func (stream *eventStream) eventWithID(id, name string, data []byte) error {
	return stream.write(fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", id, name, data))
}

func (stream *eventStream) heartbeat() error {
	return stream.write(": heartbeat\n\n")
}

// reconnect asks the client to open the stream again, which authenticates
// it again; every stream ends this way within its lifetime.
func (stream *eventStream) reconnect() error {
	return stream.write("event: control\ndata: {\"type\":\"reconnect\"}\n\n")
}

// rotationTimer fires a second before a stream's lifetime ends, while its
// write budget still has time left to announce the renewal.
func rotationTimer(lifetime time.Duration) *time.Timer {
	return time.NewTimer(max(time.Duration(0), lifetime-time.Second))
}

func (stream *eventStream) write(frame string) error {
	if err := stream.ctx.Err(); err != nil {
		return err
	}
	if len(frame) > stream.maximumFrameBytes {
		return errors.New("event frame exceeds limit")
	}
	writeDeadline := time.Now().Add(10 * time.Second)
	if !stream.deadline.IsZero() {
		writeDeadline = minTime(writeDeadline, stream.deadline)
	}
	if err := stream.controller.SetWriteDeadline(writeDeadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	defer stream.controller.SetWriteDeadline(time.Time{})
	if !stream.started {
		stream.w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		stream.w.Header().Set("Cache-Control", "no-store")
		stream.w.Header().Set("X-Accel-Buffering", "no")
		stream.started = true
	}
	if _, err := fmt.Fprint(stream.w, frame); err != nil {
		return err
	}
	return stream.controller.Flush()
}

// streamSnapshots separates subscription work from transport writes. Collection
// continues independently of slow clients; every frame gets its own deadline
// and holds at most maximumFrameBytes. encode returns nil data for a
// snapshot that needs no frame.
func streamSnapshots[T any](w http.ResponseWriter, request *http.Request, event string, lifetime time.Duration, maximumFrameBytes int,
	subscribe func(context.Context) (<-chan telemetry.SnapshotResult[T], func()),
	encode func(T) ([]byte, error),
) {
	ctx, cancel := context.WithTimeout(request.Context(), lifetime)
	defer cancel()
	deadline, _ := ctx.Deadline()
	stream, ok := newEventStream(ctx, w, deadline, maximumFrameBytes)
	if !ok {
		writeProblem(w, request, 500, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	snapshots, unsubscribe := subscribe(ctx)
	defer unsubscribe()
	unavailable := func() {
		if !stream.started {
			writeProblem(w, request, 500, event+"_snapshot_unavailable", "Snapshot unavailable", "The snapshot could not be collected.")
		}
	}
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	rotation := rotationTimer(lifetime)
	defer rotation.Stop()
	for {
		select {
		case <-ctx.Done():
			unavailable()
			return
		case result := <-snapshots:
			if result.Err != nil {
				unavailable()
				return
			}
			data, err := encode(result.Value)
			if err != nil {
				unavailable()
				return
			}
			if data == nil {
				continue
			}
			if err := stream.event(event, data); err != nil {
				unavailable()
				return
			}
		case <-heartbeat.C:
			if stream.started {
				if err := stream.heartbeat(); err != nil {
					return
				}
			}
		case <-rotation.C:
			if stream.started {
				_ = stream.reconnect()
				return
			}
		}
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
