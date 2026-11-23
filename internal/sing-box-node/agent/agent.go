// SPDX-License-Identifier: GPL-3.0-or-later

// Package agent connects a node to its panel: enrollment, the authenticated
// session and reconnection. The node dials out; it accepts no connection from
// its panel.
package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/service"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

const (
	keepaliveInterval = 30 * time.Second
	// enrollmentCheckInterval is how often a running node looks for an
	// enrollment saved while it runs.
	enrollmentCheckInterval = 3 * time.Second
	// resultCheckInterval is how often a running node looks for the result
	// of its root update helper.
	resultCheckInterval = 2 * time.Second
	keepaliveTimeout    = 10 * time.Second
	handshakeTimeout    = 15 * time.Second
	enrollTimeout       = 30 * time.Second
	minimumBackoff      = time.Second
	maximumBackoff      = time.Minute
	// maximumCommands bounds the commands a node runs at once; more wait
	// for a slot until their deadline.
	maximumCommands = 8
)

// Options supplies the agent's collaborators.
type Options struct {
	// Logf reports session changes; nil discards them.
	Logf func(format string, args ...any)
	// Connected, if set, is called after each authenticated Welcome.
	Connected func()
	// Console, if set, serves the local console beside the node's sessions
	// until its context ends; its error stops the node. Through the console
	// an unenrolled node can be enrolled, and readings shows the node's own
	// state.
	Console func(ctx context.Context, readings ReadingsSource) error
	// Update adjusts how the node updates itself when the panel asks.
	Update UpdateOptions
}

func (options Options) logf(format string, args ...any) {
	if options.Logf != nil {
		options.Logf(format, args...)
	}
}

// EnrollmentError is a code exchange with a panel that did not complete.
type EnrollmentError struct {
	Endpoint string
	// Rejected reports that the panel refused the code; otherwise the panel
	// could not be reached or could not enroll the node.
	Rejected bool
	Reason   string
}

func (err *EnrollmentError) Error() string {
	return fmt.Sprintf("enroll with %s: %s", err.Endpoint, err.Reason)
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
		switch status.Code(err) {
		case codes.InvalidArgument, codes.Unauthenticated, codes.PermissionDenied, codes.FailedPrecondition:
			return settings.Enrollment{}, &EnrollmentError{Endpoint: code.Endpoint, Rejected: true, Reason: status.Convert(err).Message()}
		}
		return settings.Enrollment{}, &EnrollmentError{Endpoint: code.Endpoint, Reason: describe(err)}
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
// runtime, and holds a session with the panel the link is enrolled with until
// ctx ends, reconnecting with backoff and starting over whenever the
// enrollment changes. An unenrolled node waits for the console to enroll it.
func Run(ctx context.Context, link *Link, options Options) (runErr error) {
	if !link.Enrolled() && options.Console == nil {
		return settings.ErrNotEnrolled
	}
	dataDir := link.dataDir
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
	instanceID, err := database.InstanceID(ctx)
	if err != nil {
		return err
	}
	// Background work stops when Run returns, for any reason; a console that
	// fails stops the node.
	ctx, stop := context.WithCancelCause(ctx)
	var background sync.WaitGroup
	defer func() { stop(nil); background.Wait() }()
	background.Go(func() { node.RunPublicIPDetection(ctx) })
	background.Go(func() { node.RunTrafficCollection(ctx) })
	// An enrollment saved by sing-box-node enroll while the node runs
	// replaces the current one, as the console's does.
	background.Go(func() { link.watch(ctx, enrollmentCheckInterval, options.logf) })
	self := newUpdates(dataDir, link.build, options.Update)
	self.load()
	background.Go(func() { self.watch(ctx, resultCheckInterval) })
	// The sessions' reports and the console share one host sampler.
	sampler := &host.Sampler{}
	if options.Console != nil {
		background.Go(func() {
			if err := options.Console(ctx, readingsOf(node, sampler, dataDir)); err != nil {
				stop(fmt.Errorf("console: %w", err))
			}
		})
	}

	slots := make(chan struct{}, maximumCommands)
	var inFlight sync.WaitGroup
	defer inFlight.Wait()
	link.supervise(ctx, func(attempt context.Context, enrollment settings.Enrollment, connected func()) error {
		connection, err := dial(enrollment.Endpoint, enrollment.Path, enrollment.Pin)
		if err != nil {
			return err
		}
		defer connection.Close()
		client := nodeapi.NewNodeServiceClient(connection)
		commands := &executor{
			node: node, database: database, fetch: fetchArtifact(client), now: time.Now, slots: slots, updates: self,
			fetchRelease: func(ctx context.Context, version, dir string) error {
				_, err := fetchRelease(ctx, client, enrollment, link.build, version, dir)
				return err
			},
		}
		identity := sessionIdentity{enrollment: enrollment, instanceID: instanceID, build: link.build}
		return holdSession(attempt, ctx, client, identity, commands, sampler, dataDir, &inFlight, func() {
			options.logf("connected to panel %s as node %s", enrollment.Endpoint, enrollment.NodeID)
			connected()
			if options.Connected != nil {
				options.Connected()
			}
		})
	}, options.logf)
	// Only a failed console ends the node with an error.
	if cause := context.Cause(ctx); !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return nil
}

// sessionIdentity is who the node is to its panel.
type sessionIdentity struct {
	enrollment settings.Enrollment
	instanceID string
	build      buildinfo.Info
}

// holdSession authenticates one session, reports the node's state and serves
// its commands until it or attempt ends, and calls connected once it serves
// commands. Commands run on lifetime, not on the session: a command in flight
// finishes even when the session drops, and the panel learns its outcome from
// the node's state.
func holdSession(attempt, lifetime context.Context, client nodeapi.NodeServiceClient, identity sessionIdentity, commands *executor,
	sampler *host.Sampler, dataDir string, inFlight *sync.WaitGroup, connected func(),
) error {
	enrollment := identity.enrollment
	ctx, cancel := context.WithCancel(attempt)
	defer cancel()
	stream, err := client.Connect(ctx)
	if err != nil {
		return err
	}
	handshake := time.AfterFunc(handshakeTimeout, cancel)
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Hello{Hello: &nodeapi.Hello{
		NodeId: enrollment.NodeID, Protocol: protocol(), BuildVersion: identity.build.Version, Platform: platform(),
		InstanceId: identity.instanceID,
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
	welcome := frame.GetWelcome()
	if welcome == nil {
		return errors.New("the panel did not welcome the session")
	}
	if !handshake.Stop() {
		return errors.New("handshake timed out")
	}
	var sendMu sync.Mutex
	send := func(frame *nodeapi.ConnectRequest) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(frame)
	}
	// The panel accepts commands once it has the node's state.
	reports := newReporter(commands.node, commands.database, sampler, dataDir, send, welcome.GetRuntimeHistoryAfter())
	reports.updates = commands.updates
	if err := reports.start(ctx); err != nil {
		return fmt.Errorf("report state: %w", err)
	}
	connected()
	reporting := make(chan error, 1)
	go func() { reporting <- reports.run(ctx, welcome.GetMetricsInterval().AsDuration()) }()
	defer func() { cancel(); <-reporting }()
	frames := make(chan *nodeapi.ConnectResponse)
	received := make(chan error, 1)
	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				received <- err
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	var runningMu sync.Mutex
	running := map[string]context.CancelFunc{}
	for {
		var frame *nodeapi.ConnectResponse
		select {
		case err := <-received:
			return err
		case err := <-reporting:
			reporting <- err
			if err == nil {
				err = ctx.Err()
			}
			return fmt.Errorf("report state: %w", err)
		case frame = <-frames:
		}
		switch {
		case frame.GetTelemetryControl() != nil:
			reports.setInterval(frame.GetTelemetryControl().GetMetricsInterval().AsDuration())
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
				if nodeapi.ChangesState(command.GetBody()) {
					// The panel sees the state a command left before its result.
					_ = reports.reportChange(ctx, command.GetRequestId())
				}
				_ = send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: result}})
				if followUp := commands.takeFollowUp(command.GetRequestId()); followUp != nil {
					followUp()
				}
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
