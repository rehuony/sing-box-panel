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
	"io"
	mathrand "math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/service"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
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

// Run starts the node: it opens the node database, starts and recovers the
// runtime, and holds a session with the panel the node in dataDir is enrolled
// with until ctx ends, reconnecting with backoff.
func Run(ctx context.Context, dataDir string, options Options) (runErr error) {
	enrollment, err := settings.Load(dataDir)
	if err != nil {
		return err
	}
	lease, err := procctl.AcquireLease(dataDir)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, lease.Close()) }()
	database, err := store.Open(ctx, filepath.Join(dataDir, "node.db"))
	if err != nil {
		return fmt.Errorf("open node database: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, database.Close()) }()
	node, err := service.New(database, service.Options{DataDir: dataDir, Events: func(event service.Event) {
		options.logf("%s %s: %s %v", event.Level, event.Code, event.Message, event.Metadata)
	}})
	if err != nil {
		return err
	}
	runtime, err := node.Start(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer func() { runErr = errors.Join(runErr, runtime.Close()) }()
	runtime.StartRecovery(ctx)
	detection := make(chan struct{})
	go func() { defer close(detection); node.RunPublicIPDetection(ctx) }()
	defer func() { <-detection }()

	connection, err := dial(enrollment.Endpoint, enrollment.Path, enrollment.Pin)
	if err != nil {
		return err
	}
	defer connection.Close()
	client := nodeapi.NewNodeServiceClient(connection)
	commands := &executor{node: node, database: database, fetch: fetchArtifact(client), now: time.Now}
	var inFlight sync.WaitGroup
	defer inFlight.Wait()
	backoff := minimumBackoff
	for {
		started := time.Now()
		err := holdSession(ctx, client, enrollment, commands, &inFlight, options)
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

// holdSession authenticates one session and serves its commands until it
// ends. Commands run on lifetime, not on the session: a command in flight
// finishes even when the session drops, and the panel learns its outcome
// from the node's state.
func holdSession(lifetime context.Context, client nodeapi.NodeServiceClient, enrollment settings.Enrollment, commands *executor, inFlight *sync.WaitGroup, options Options) error {
	ctx, cancel := context.WithCancel(lifetime)
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
	var sendMu sync.Mutex
	send := func(frame *nodeapi.ConnectRequest) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(frame)
	}
	var runningMu sync.Mutex
	running := map[string]context.CancelFunc{}
	for {
		frame, err := stream.Recv()
		if err != nil {
			return err
		}
		switch {
		case frame.GetCommand() != nil:
			command := frame.GetCommand()
			commandContext, stop := context.WithCancel(lifetime)
			runningMu.Lock()
			running[command.GetRequestId()] = stop
			runningMu.Unlock()
			inFlight.Add(1)
			go func() {
				defer inFlight.Done()
				defer func() {
					runningMu.Lock()
					delete(running, command.GetRequestId())
					runningMu.Unlock()
					stop()
				}()
				result := commands.execute(commandContext, command)
				_ = send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: result}})
			}()
		case frame.GetCancel() != nil:
			runningMu.Lock()
			if stop := running[frame.GetCancel().GetRequestId()]; stop != nil {
				stop()
			}
			runningMu.Unlock()
		}
	}
}

// fetchArtifact streams a cached core artifact the panel granted.
func fetchArtifact(client nodeapi.NodeServiceClient) func(context.Context, string, string, io.Writer) (int64, error) {
	return func(ctx context.Context, grant, artifactID string, destination io.Writer) (int64, error) {
		stream, err := client.FetchArtifact(ctx, &nodeapi.FetchArtifactRequest{Grant: grant, ArtifactId: artifactID})
		if err != nil {
			return 0, err
		}
		var total int64
		for {
			chunk, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			if err != nil {
				return total, err
			}
			written, err := destination.Write(chunk.GetData())
			total += int64(written)
			if err != nil {
				return total, err
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
