// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 60 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 64 << 10
	shutdownTimeout   = 5 * time.Second
)

// Listen opens the console's listener on address, a host:port.
func Listen(address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	return listener, nil
}

// Serve serves handler on listener until ctx ends, then shuts down. Requests
// run on ctx, so an enrollment in progress stops with the node.
func Serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: readHeaderTimeout, ReadTimeout: readTimeout,
		IdleTimeout: idleTimeout, MaxHeaderBytes: maxHeaderBytes,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	err := server.Shutdown(shutdown)
	if serveErr := <-served; !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	return err
}
