// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (handler *Handler) streamMetrics(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	_, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, request, 500, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	// Include collection in the reconnect budget, including the first snapshot.
	ctx, cancel := context.WithTimeout(request.Context(), time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	controller := http.NewResponseController(w)
	started := false
	writeSample := func() error {
		metrics, err := handler.commands.Metrics(ctx)
		if err != nil {
			return err
		}
		runtime, err := handler.commands.RuntimeStatus(ctx)
		if err != nil {
			return err
		}
		data, err := json.Marshal(map[string]any{"metrics": metrics, "runtime": runtime})
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		writeDeadline := time.Now().Add(10 * time.Second)
		if deadline.Before(writeDeadline) {
			writeDeadline = deadline
		}
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
		if _, err := fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := writeSample(); err != nil {
		if !started {
			writeProblem(w, request, http.StatusInternalServerError, "metrics_snapshot_unavailable", "Metrics unavailable", "The metrics snapshot could not be collected.")
		}
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	// Periodic reconnect rechecks the authenticated session after token rotation.
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := writeSample(); err != nil {
				return
			}
		}
	}
}
