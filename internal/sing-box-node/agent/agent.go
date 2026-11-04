// SPDX-License-Identifier: GPL-3.0-or-later

// Package agent connects a node to its panel: enrollment, the authenticated
// session and reconnection. The node dials out; it never listens.
package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"net"
	"os"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
)

const (
	keepaliveInterval = 30 * time.Second
	keepaliveTimeout  = 10 * time.Second
	handshakeTimeout  = 15 * time.Second
	enrollTimeout     = 30 * time.Second
	minimumBackoff    = time.Second
	maximumBackoff    = time.Minute
)

// Options supplies the agent's collaborators.
type Options struct {
	Build buildinfo.Info
	// Logf reports session changes; nil discards them.
	Logf func(format string, args ...any)
	// Connected, if set, is called after each authenticated Welcome.
	Connected func()
}

func (options Options) logf(format string, args ...any) {
	if options.Logf != nil {
		options.Logf(format, args...)
	}
}

// Enroll exchanges an enrollment code for a node identity with a new key and
// saves it in dataDir, replacing any previous enrollment.
func Enroll(ctx context.Context, dataDir, rawCode string, build buildinfo.Info) (settings.Enrollment, error) {
	code, err := nodeapi.ParseEnrollmentCode(rawCode)
	if err != nil {
		return settings.Enrollment{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return settings.Enrollment{}, err
	}
	connection, err := dial(code.Endpoint, code.Path, code.Pin)
	if err != nil {
		return settings.Enrollment{}, err
	}
	defer connection.Close()
	hostname, _ := os.Hostname()
	ctx, cancel := context.WithTimeout(ctx, enrollTimeout)
	defer cancel()
	response, err := nodeapi.NewNodeServiceClient(connection).Enroll(ctx, &nodeapi.EnrollRequest{
		Token: code.Token, PublicKey: publicKey, Protocol: protocol(), BuildVersion: build.Version,
		Platform: platform(), Hostname: hostname,
	})
	if err != nil {
		return settings.Enrollment{}, fmt.Errorf("enroll with %s: %s", code.Endpoint, describe(err))
	}
	enrollment := settings.Enrollment{
		Endpoint: code.Endpoint, Path: code.Path, Pin: code.Pin, PanelID: response.GetPanelId(), NodeID: response.GetNodeId(), Key: privateKey,
	}
	if err := settings.Save(dataDir, enrollment); err != nil {
		return settings.Enrollment{}, fmt.Errorf("the panel enrolled node %s but saving the enrollment failed; issue a new code: %w", enrollment.NodeID, err)
	}
	return enrollment, nil
}

// Run holds a session with the panel the node in dataDir is enrolled with
// until ctx ends, reconnecting with backoff.
func Run(ctx context.Context, dataDir string, options Options) error {
	enrollment, err := settings.Load(dataDir)
	if err != nil {
		return err
	}
	connection, err := dial(enrollment.Endpoint, enrollment.Path, enrollment.Pin)
	if err != nil {
		return err
	}
	defer connection.Close()
	client := nodeapi.NewNodeServiceClient(connection)
	backoff := minimumBackoff
	for {
		started := time.Now()
		err := holdSession(ctx, client, enrollment, options)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > maximumBackoff {
			backoff = minimumBackoff
		}
		options.logf("session with panel %s ended: %s; reconnecting in %s", enrollment.Endpoint, describe(err), backoff.Round(time.Second))
		wait := backoff/2 + mathrand.N(backoff/2+1)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		backoff = min(2*backoff, maximumBackoff)
	}
}

// holdSession authenticates one session and serves it until it ends.
func holdSession(ctx context.Context, client nodeapi.NodeServiceClient, enrollment settings.Enrollment, options Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.Connect(ctx)
	if err != nil {
		return err
	}
	handshake := time.AfterFunc(handshakeTimeout, cancel)
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Hello{Hello: &nodeapi.Hello{
		NodeId: enrollment.NodeID, Protocol: protocol(), BuildVersion: options.Build.Version, Platform: platform(),
	}}}); err != nil {
		return err
	}
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	challenge := frame.GetChallenge()
	if challenge == nil {
		return errors.New("the panel did not send a challenge")
	}
	if challenge.GetPanelId() != enrollment.PanelID {
		return fmt.Errorf("the panel identifies as %q, not the enrolled panel %q", challenge.GetPanelId(), enrollment.PanelID)
	}
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Proof{
		Proof: nodeapi.SignProof(enrollment.Key, enrollment.PanelID, enrollment.NodeID, challenge.GetNonce()),
	}}); err != nil {
		return err
	}
	frame, err = stream.Recv()
	if err != nil {
		return err
	}
	if frame.GetWelcome() == nil {
		return errors.New("the panel did not welcome the session")
	}
	if !handshake.Stop() {
		return errors.New("handshake timed out")
	}
	options.logf("connected to panel %s as node %s", enrollment.Endpoint, enrollment.NodeID)
	if options.Connected != nil {
		options.Connected()
	}
	for {
		frame, err := stream.Recv()
		if err != nil {
			return err
		}
		if command := frame.GetCommand(); command != nil {
			// This node build carries out no commands yet.
			if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: &nodeapi.Result{
				RequestId: command.GetRequestId(),
				Outcome: &nodeapi.Result_Problem{Problem: &nodeapi.Problem{
					Code: "command_unsupported", Title: "Command unsupported", Detail: "This node does not carry out commands yet.",
				}},
			}}}); err != nil {
				return err
			}
		}
	}
}

// dial prepares a connection to the panel. A pin replaces certificate-chain
// verification with a check of the panel certificate's public key. A path
// prefix, the panel's base path, goes before every method path, so a
// reverse proxy can route the node service beside the Web application.
func dial(endpoint, path, pin string) (*grpc.ClientConn, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS13}
	if pin != "" {
		// The pin below authenticates the self-signed listener certificate.
		config.InsecureSkipVerify = true
		config.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 || nodeapi.PublicKeyPin(state.PeerCertificates[0].RawSubjectPublicKeyInfo) != pin {
				return errors.New("the panel certificate does not match the enrolled pin")
			}
			return nil
		}
	} else {
		host, _, err := net.SplitHostPort(endpoint)
		if err != nil {
			return nil, err
		}
		config.ServerName = host
	}
	return grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(config)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: keepaliveInterval, Timeout: keepaliveTimeout, PermitWithoutStream: true}),
		grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, request, reply any, connection *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
			return invoke(ctx, path+method, request, reply, connection, options...)
		}),
		grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, connection *grpc.ClientConn, method string, stream grpc.Streamer, options ...grpc.CallOption) (grpc.ClientStream, error) {
			return stream(ctx, desc, connection, path+method, options...)
		}),
	)
}

func protocol() *nodeapi.ProtocolVersion {
	return &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor, Minor: nodeapi.ProtocolMinor}
}

func platform() *nodeapi.Platform {
	return &nodeapi.Platform{Os: runtime.GOOS, Arch: runtime.GOARCH}
}

// describe renders a gRPC failure as its status code and message.
func describe(err error) string {
	if err == nil {
		return "closed"
	}
	if value, ok := status.FromError(err); ok {
		return fmt.Sprintf("%s: %s", value.Code(), value.Message())
	}
	return err.Error()
}
