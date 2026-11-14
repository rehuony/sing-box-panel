// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

const (
	// keepaliveInterval and keepaliveTimeout detect dead sessions.
	keepaliveInterval = 30 * time.Second
	keepaliveTimeout  = 10 * time.Second
	// maximumMessageBytes bounds one frame from a node.
	maximumMessageBytes = 4 << 20
	stopTimeout         = 5 * time.Second
)

// Serve accepts node sessions on listeners until ctx ends, then ends every
// session and stops. The node service sits under the panel's base path, so a
// reverse proxy can route it beside the Web application.
func (service *Service) Serve(ctx context.Context, listeners NodeListeners) error {
	closeListeners := func(err error) error {
		return errors.Join(err, listeners.TLS.Close(), listeners.Cleartext.Close())
	}
	identity, err := service.Identity()
	if err != nil {
		return closeListeners(err)
	}
	// No session survives the process that held it.
	if err := service.database.ClearNodeSessions(ctx); err != nil {
		// A stop requested while the service starts is not a failure.
		if ctx.Err() != nil && errors.Is(err, context.Canceled) {
			return closeListeners(nil)
		}
		return closeListeners(err)
	}
	options := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: keepaliveInterval, Timeout: keepaliveTimeout}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: keepaliveInterval / 2, PermitWithoutStream: true}),
		grpc.MaxRecvMsgSize(maximumMessageBytes),
		grpc.ConnectionTimeout(handshakeTimeout),
	}
	secure := grpc.NewServer(append(options, grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{identity.Certificate}, MinVersion: tls.VersionTLS13,
	})))...)
	cleartext := grpc.NewServer(options...)
	desc := NodeServiceDesc(service.settings.Value().Server.BasePath)
	implementation := &nodeServer{service: service}
	secure.RegisterService(desc, implementation)
	cleartext.RegisterService(desc, implementation)

	served := make(chan error, 2)
	go func() { served <- secure.Serve(listeners.TLS) }()
	go func() { served <- cleartext.Serve(listeners.Cleartext) }()
	var results []error
	select {
	case err := <-served:
		results = append(results, err)
	case <-ctx.Done():
	}
	service.sessions.closeAll(errPanelStopping)
	stopped := make(chan struct{})
	go func() {
		secure.GracefulStop()
		cleartext.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(stopTimeout):
		secure.Stop()
		cleartext.Stop()
		<-stopped
	}
	for len(results) < 2 {
		results = append(results, <-served)
	}
	for _, err := range results {
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
	}
	return nil
}

// NodeServiceDesc returns the node service under basePath: with base path
// /panel, nodes call /panel/sbp.node.v1.NodeService/Connect. gRPC splits a
// request path at its last slash, so the prefix becomes part of the service
// name.
func NodeServiceDesc(basePath string) *grpc.ServiceDesc {
	desc := nodeapi.NodeService_ServiceDesc
	if basePath != "" {
		desc.ServiceName = strings.TrimPrefix(basePath, "/") + "/" + desc.ServiceName
	}
	return &desc
}
