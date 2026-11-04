// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

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
