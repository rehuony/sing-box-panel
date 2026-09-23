// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func recordOperationalLog(commands *application.Application, request application.LogRecordRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = commands.RecordLog(ctx, request)
}

func mustLogMetadata(value map[string]any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func stopHTTPServer(server *http.Server, serveResult <-chan error) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownContext)
	if shutdownErr != nil {
		_ = server.Close()
	}
	serveErr := <-serveResult
	if shutdownErr != nil {
		return fmt.Errorf("shut down panel HTTP server: %w", shutdownErr)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve panel HTTP server during shutdown: %w", serveErr)
	}
	return nil
}
