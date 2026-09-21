// SPDX-License-Identifier: GPL-3.0-or-later
package panelprocess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

var ErrUnavailable = errors.New("panel runtime is unavailable")

// SetRuntimeHandler attaches the server's synchronous core controls to the
// existing owner-only (0600) socket. It is configured once before Ready.
func (c *Control) SetRuntimeHandler(handler http.Handler) {
	c.server.Handler.(*http.ServeMux).Handle("POST /runtime", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		ready := c.status.State == "ready"
		c.mu.Unlock()
		if !ready {
			http.Error(w, "panel runtime is not ready", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
}

func CallRuntime(ctx context.Context, dataDir string, input any) (json.RawMessage, error) {
	socketPath := filepath.Join(dataDir, socketName)
	info, err := os.Lstat(socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w; start it with server start: %w", ErrUnavailable, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil, errors.New("panel control path is not a socket")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socketPath)
	}}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://panel/runtime", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: contact panel runtime: %w", ErrUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("panel runtime returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, errors.New("panel runtime returned invalid JSON")
	}
	return raw, nil
}
