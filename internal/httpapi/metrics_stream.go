// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"encoding/json"
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
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, request, 500, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	writeSample := func() error {
		metrics, err := handler.commands.Metrics(request.Context())
		if err != nil {
			return err
		}
		runtime, err := handler.commands.RuntimeStatus(request.Context())
		if err != nil {
			return err
		}
		data, err := json.Marshal(map[string]any{"metrics": metrics, "runtime": runtime})
		if err != nil {
			return err
		}
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && err != http.ErrNotSupported {
			return err
		}
		_, err = fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data)
		flusher.Flush()
		return err
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := writeSample(); err != nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	// Periodic reconnect rechecks the authenticated session after token rotation.
	expiry := time.NewTimer(time.Minute)
	defer expiry.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-expiry.C:
			return
		case <-ticker.C:
			if err := writeSample(); err != nil {
				return
			}
		}
	}
}
