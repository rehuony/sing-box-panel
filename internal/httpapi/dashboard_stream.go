// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	dashboardStreamInterval = 30 * time.Second
	dashboardStreamLifetime = time.Minute
)

func (handler *Handler) streamDashboard(w http.ResponseWriter, request *http.Request) {
	handler.streamDashboardWithSchedule(w, request, dashboardStreamInterval, dashboardStreamLifetime)
}

func (handler *Handler) streamDashboardWithSchedule(
	w http.ResponseWriter,
	request *http.Request,
	interval time.Duration,
	lifetime time.Duration,
) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, request, http.StatusInternalServerError, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	writeSnapshot := func() error {
		snapshot, err := handler.commands.DashboardSnapshot(request.Context())
		if err != nil {
			return err
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: dashboard\ndata: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := writeSnapshot(); err != nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	expiry := time.NewTimer(lifetime)
	defer expiry.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-expiry.C:
			return
		case <-ticker.C:
			if err := writeSnapshot(); err != nil {
				return
			}
		}
	}
}
