// SPDX-License-Identifier: GPL-3.0-or-later

// Package panelprocess controls a local panel process without an OS service manager.
package panelprocess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const socketName = "panel-control.sock"

// Status describes the live process, never a persisted PID file.
type Status struct {
	State        string    `json:"state"`
	PID          int       `json:"pid,omitempty"`
	Version      string    `json:"version,omitempty"`
	StartedAt    time.Time `json:"started_at,omitzero"`
	SettingsPath string    `json:"settings_path,omitempty"`
	DataDir      string    `json:"data_dir"`
	Listen       string    `json:"listen,omitempty"`
	ManagedBy    string    `json:"managed_by,omitempty"`
}

type response struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Control is owned by the server's data-directory lease holder.
type Control struct {
	listener net.Listener
	server   *http.Server
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	status   Status
	result   error
}

// Listen must be called while holding the exclusive data-directory lease.
// Only that owner may remove a socket left behind by an earlier crash.
func Listen(status Status, cancel context.CancelFunc) (*Control, error) {
	path := filepath.Join(status.DataDir, socketName)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("panel control path is not a socket: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen for panel control (use a shorter data directory if the Unix socket path is too long): %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	status.State = "starting"
	status.PID = os.Getpid()
	status.StartedAt = time.Now().UTC()
	status.ManagedBy = "terminal"
	if os.Getenv("SING_BOX_PANEL_SUPERVISOR") == "systemd" {
		status.ManagedBy = "systemd"
	}
	control := &Control{listener: listener, cancel: cancel, done: make(chan struct{}), status: status}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", control.serveStatus)
	mux.HandleFunc("POST /stop", control.serveStop)
	control.server = &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096}
	go func() { _ = control.server.Serve(listener) }()
	return control, nil
}

func (control *Control) Ready(listen string) {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.status.State == "starting" {
		control.status.State = "ready"
	}
	control.status.Listen = listen
}

// Stopping marks signal-driven shutdown as well as local stop requests.
func (control *Control) Stopping() {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.status.State != "stopped" {
		control.status.State = "stopping"
	}
}

func (control *Control) serveStatus(writer http.ResponseWriter, _ *http.Request) {
	control.mu.Lock()
	status := control.status
	control.mu.Unlock()
	writeResponse(writer, response{Status: status})
}

func (control *Control) serveStop(writer http.ResponseWriter, request *http.Request) {
	control.mu.Lock()
	status := control.status
	if status.ManagedBy == "systemd" {
		control.mu.Unlock()
		writeResponse(writer, response{Status: status, Error: "panel is managed by systemd; use systemd stop"})
		return
	}
	if control.status.State != "stopped" {
		control.status.State = "stopping"
	}
	control.mu.Unlock()
	control.cancel()
	select {
	case <-request.Context().Done():
		return
	case <-control.done:
	}
	control.mu.Lock()
	result := response{Status: control.status}
	if control.result != nil {
		result.Error = control.result.Error()
	}
	control.mu.Unlock()
	writeResponse(writer, result)
}

func writeResponse(writer http.ResponseWriter, value response) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

// StopAccepting removes the socket before releasing the data-directory lease.
func (control *Control) StopAccepting() { _ = control.listener.Close() }

// Finish acknowledges stop only after resource cleanup and lease release.
// A disconnected or crashed server therefore cannot masquerade as a clean stop.
func (control *Control) Finish(err error) {
	control.mu.Lock()
	control.result = err
	control.status.State = "stopped"
	control.mu.Unlock()
	close(control.done)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = control.server.Shutdown(ctx)
	_ = control.server.Close()
}

func request(ctx context.Context, dataDir, method, path string) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	socketPath := filepath.Join(dataDir, socketName)
	info, err := os.Lstat(socketPath)
	if errors.Is(err, os.ErrNotExist) {
		return Status{State: "stopped", DataDir: dataDir}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return Status{}, errors.New("panel control path is not a socket")
	}
	dialer := &net.Dialer{}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socketPath)
	}}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, method, "http://panel"+path, nil)
	if err != nil {
		return Status{}, err
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return Status{State: "stopped", DataDir: dataDir}, nil
		}
		return Status{}, fmt.Errorf("contact panel process: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("panel control returned HTTP %d", resp.StatusCode)
	}
	var value response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&value); err != nil {
		return Status{}, fmt.Errorf("read panel control response: %w", err)
	}
	if value.Error != "" {
		return value.Status, errors.New(value.Error)
	}
	return value.Status, nil
}

func Inspect(ctx context.Context, dataDir string) (Status, error) {
	return request(ctx, dataDir, http.MethodGet, "/status")
}
func Stop(ctx context.Context, dataDir string) (Status, error) {
	status, err := request(ctx, dataDir, http.MethodPost, "/stop")
	if errors.Is(err, context.DeadlineExceeded) {
		return status, fmt.Errorf("stop wait timed out; shutdown may still be in progress, check server status or retry server stop: %w", err)
	}
	return status, err
}
