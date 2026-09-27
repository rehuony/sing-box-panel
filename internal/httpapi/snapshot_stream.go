// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
)

const maximumSnapshotFrameBytes = 1 << 20

// streamSnapshots separates subscription work from transport writes. Collection
// continues independently of slow clients; every frame gets its own deadline.
func streamSnapshots[T any](w http.ResponseWriter, request *http.Request, event string, lifetime time.Duration,
	subscribe func(context.Context) (<-chan application.SnapshotResult[T], func()),
	encode func(T) ([]byte, error),
) {
	if _, ok := w.(http.Flusher); !ok {
		writeProblem(w, request, 500, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), lifetime)
	defer cancel()
	deadline, _ := ctx.Deadline()
	snapshots, unsubscribe := subscribe(ctx)
	defer unsubscribe()
	controller := http.NewResponseController(w)
	started := false
	write := func(frame string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(frame) > maximumSnapshotFrameBytes {
			return errors.New("snapshot frame exceeds limit")
		}
		writeDeadline := minTime(time.Now().Add(10*time.Second), deadline)
		if err := controller.SetWriteDeadline(writeDeadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		defer controller.SetWriteDeadline(time.Time{})
		if !started {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		if _, err := fmt.Fprint(w, frame); err != nil {
			return err
		}
		return controller.Flush()
	}
	unavailable := func() {
		if !started {
			writeProblem(w, request, 500, event+"_snapshot_unavailable", "Snapshot unavailable", "The snapshot could not be collected.")
		}
	}
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	// Announce renewal while the authentication/write budget still has time left.
	rotation := time.NewTimer(max(time.Duration(0), lifetime-time.Second))
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
			if err := write(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)); err != nil {
				unavailable()
				return
			}
		case <-heartbeat.C:
			if started {
				if err := write(": heartbeat\n\n"); err != nil {
					return
				}
			}
		case <-rotation.C:
			if started {
				_ = write("event: control\ndata: {\"type\":\"reconnect\"}\n\n")
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
