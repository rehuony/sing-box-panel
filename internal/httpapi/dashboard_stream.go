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

const (
	dashboardStreamInterval = 30 * time.Second
	dashboardStreamLifetime = time.Minute
)

func (handler *Handler) streamDashboard(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	streamDashboardSnapshots(w, request, handler.commands.DashboardSnapshot, dashboardStreamInterval, dashboardStreamLifetime)
}

func streamDashboardSnapshots(
	w http.ResponseWriter,
	request *http.Request,
	load func(context.Context) (application.DashboardStreamSnapshot, error),
	interval time.Duration,
	lifetime time.Duration,
) {
	if _, ok := w.(http.Flusher); !ok {
		writeProblem(w, request, http.StatusInternalServerError, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	// The reconnect budget includes the initial query and every later query.
	ctx, cancel := context.WithTimeout(request.Context(), lifetime)
	defer cancel()
	deadline, _ := ctx.Deadline()
	controller := http.NewResponseController(w)
	streamStarted := false
	loadSnapshot := func() ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapshot, err := load(ctx)
		if err != nil {
			return nil, err
		}
		data, err := encodeDashboardSnapshot(snapshot)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, context.DeadlineExceeded
		}
		return data, nil
	}
	writeSnapshot := func(data []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
		writeDeadline := time.Now().Add(10 * time.Second)
		if deadline.Before(writeDeadline) {
			writeDeadline = deadline
		}
		if err := controller.SetWriteDeadline(writeDeadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		// A write deadline must not remain armed during the 30-second idle gap
		// or prevent net/http from writing the normal end of the response.
		defer controller.SetWriteDeadline(time.Time{})
		if !streamStarted {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Accel-Buffering", "no")
			streamStarted = true
		}
		if _, err := fmt.Fprintf(w, "event: dashboard\ndata: %s\n\n", data); err != nil {
			return err
		}
		return controller.Flush()
	}

	initial, err := loadSnapshot()
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "dashboard_snapshot_unavailable", "Dashboard unavailable", "The dashboard snapshot could not be collected.")
		return
	}
	if err := writeSnapshot(initial); err != nil {
		if !streamStarted {
			writeProblem(w, request, http.StatusInternalServerError, "dashboard_snapshot_unavailable", "Dashboard unavailable", "The dashboard snapshot could not be sent.")
		}
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !time.Now().Before(deadline) {
				return
			}
			data, err := loadSnapshot()
			if err != nil {
				return
			}
			if err := writeSnapshot(data); err != nil {
				return
			}
		}
	}
}
