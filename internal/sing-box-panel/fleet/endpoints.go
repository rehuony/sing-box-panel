// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"net"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

// Open readies the node endpoints; call it before the Web server serves
// them. No session survives the process that held it.
func (service *Service) Open(ctx context.Context) error {
	return service.database.ClearNodeSessions(ctx)
}

// Shutdown ends every node exchange, telling each node that the panel is
// stopping, and waits for them to finish until ctx ends. The endpoints
// refuse exchanges from then on.
func (service *Service) Shutdown(ctx context.Context) {
	service.exchangesMu.Lock()
	service.stop()
	service.exchangesMu.Unlock()
	finished := make(chan struct{})
	go func() {
		service.exchanges.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
	}
}

// ServeNode serves the node endpoint at path, a nodeapi path relative to the
// panel's base path. The panel speaks plain HTTP, so it accepts nodes only
// from its own host: a node there, or a reverse proxy there that terminates
// the HTTPS of nodes elsewhere. Node traffic never crosses a network
// unencrypted.
func (service *Service) ServeNode(w http.ResponseWriter, request *http.Request, path string) {
	server := &nodeServer{service: service}
	var exchange func(http.ResponseWriter, *http.Request, context.Context)
	switch path {
	case nodeapi.EnrollPath:
		exchange = serve(func(stream *nodeapi.EnrollServer) error {
			enrollment, err := stream.RecvWithin(handshakeTimeout)
			if err != nil {
				return err
			}
			response, err := server.Enroll(stream.Context(), enrollment)
			if err != nil {
				return err
			}
			return stream.Send(response)
		})
	case nodeapi.ConnectPath:
		exchange = serve(server.Connect)
	case nodeapi.ArtifactPath:
		exchange = serve(func(stream *nodeapi.ArtifactServer) error {
			fetch, err := stream.RecvWithin(handshakeTimeout)
			if err != nil {
				return err
			}
			return server.FetchArtifact(fetch, stream)
		})
	case nodeapi.ReleasePath:
		exchange = serve(server.FetchRelease)
	default:
		http.NotFound(w, request)
		return
	}
	if !loopbackPeer(request.RemoteAddr) {
		http.Error(w, "Nodes reach this panel through an HTTPS reverse proxy on its host.", http.StatusForbidden)
		return
	}
	service.exchangesMu.Lock()
	stopped := service.stopped.Err() != nil
	if !stopped {
		service.exchanges.Add(1)
	}
	service.exchangesMu.Unlock()
	if stopped {
		http.Error(w, "The panel is stopping.", http.StatusServiceUnavailable)
		return
	}
	defer service.exchanges.Done()
	exchange(w, request, service.stopped)
}

// serve runs exchange on the stream a request opens and closes the stream
// with its outcome, or with the panel's stop when that comes first.
func serve[Out, In proto.Message](exchange func(*nodeapi.Stream[Out, In]) error) func(http.ResponseWriter, *http.Request, context.Context) {
	return func(w http.ResponseWriter, request *http.Request, stopped context.Context) {
		stream, err := nodeapi.Accept[Out, In](w, request)
		if err != nil {
			return
		}
		stop := context.AfterFunc(stopped, func() { _ = stream.Close(sessionEnd(errPanelStopping)) })
		defer stop()
		_ = stream.Close(exchange(stream))
	}
}

func loopbackPeer(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
