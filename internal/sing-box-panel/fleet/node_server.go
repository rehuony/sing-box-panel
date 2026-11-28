// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

const (
	// handshakeTimeout bounds each handshake step of a session.
	handshakeTimeout = 10 * time.Second
	// validationInterval is how often a session re-checks that its node is
	// still enrolled with the key it authenticated with, so a node removed by
	// another process loses its session.
	validationInterval = 30 * time.Second
	// firstReportTimeout bounds the wait for a session's first state report,
	// which opens it to commands.
	firstReportTimeout = 30 * time.Second
	// MetricsInterval is the baseline interval between metrics samples. It
	// matches the spacing of stored traffic samples, so history charts get
	// one interval per sample.
	MetricsInterval = 10 * time.Second
)

// nodeServer serves sbp.node.v1 to nodes.
type nodeServer struct {
	service *Service
}

func (server *nodeServer) Enroll(ctx context.Context, request *nodeapi.EnrollRequest) (*nodeapi.EnrollResponse, error) {
	service := server.service
	if err := checkProtocol(request.GetProtocol()); err != nil {
		return nil, err
	}
	if len(request.GetPublicKey()) != ed25519.PublicKeySize || !boundedText(request.GetHostname(), 256) ||
		!boundedText(request.GetBuildVersion(), 256) || !validPlatform(request.GetPlatform()) {
		return nil, nodeapi.Errorf(nodeapi.CodeInvalidArgument, "enrollment request is malformed")
	}
	now := service.now().UTC()
	// A node the code creates starts with the empty document, saved with it.
	initial, err := store.NewConfigurationFileUpdate(service.random, now, 0, string(configuration.Empty().CanonicalJSON()))
	if err != nil {
		return nil, nodeapi.Errorf(nodeapi.CodeInternal, "prepare configuration")
	}
	node, created, err := service.database.EnrollNode(ctx, nodeapi.TokenHash(request.GetToken()), store.NodeEnrollment{
		PublicKey: request.GetPublicKey(), Hostname: request.GetHostname(),
		OS: request.GetPlatform().GetOs(), Arch: request.GetPlatform().GetArch(),
		BuildVersion: request.GetBuildVersion(), ProtocolMinor: int(request.GetProtocol().GetMinor()),
		EnrolledAt: now, Configuration: initial,
	})
	switch {
	case errors.Is(err, store.ErrEnrollmentInvalid):
		return nil, nodeapi.Errorf(nodeapi.CodeUnauthenticated, "enrollment code is invalid, used or expired")
	case errors.Is(err, store.ErrNodeNameConflict):
		return nil, nodeapi.Errorf(nodeapi.CodeFailedPrecondition, "another node already uses the name this code was issued for")
	case err != nil:
		return nil, nodeapi.Errorf(nodeapi.CodeInternal, "record enrollment")
	}
	service.expireStatus()
	if created {
		service.records.RecordOperation(ctx, "node.create", "Node creation", nil, telemetry.OperationLogContext{NodeID: node.ID})
	} else {
		// A key replaced by enrolling again must not keep its session.
		service.sessions.close(node.ID, errNodeRemoved)
	}
	panelID, err := service.PanelID(ctx)
	if err != nil {
		return nil, nodeapi.Errorf(nodeapi.CodeInternal, "read panel identity")
	}
	service.recordNodeEvent(store.LogLevelInfo, "node.enrolled", "Node enrolled", node.ID)
	return &nodeapi.EnrollResponse{NodeId: node.ID, PanelId: panelID}, nil
}

// Connect authenticates the node with a signed challenge and then holds its
// session until the stream ends, the session is replaced or the node is
// removed.
func (server *nodeServer) Connect(stream *nodeapi.ConnectServer) error {
	service := server.service
	ctx, cancel := context.WithCancelCause(stream.Context())
	defer cancel(nil)
	receiver := receiveFrames(ctx, stream.Recv)
	// A node that stops answering ends its session, whatever proxies stand
	// between it and the panel.
	stream.KeepAlive()

	frame, err := receiver.next()
	if err != nil {
		return err
	}
	hello := frame.GetHello()
	if hello == nil {
		return nodeapi.Errorf(nodeapi.CodeInvalidArgument, "the session must start with Hello")
	}
	if err := checkProtocol(hello.GetProtocol()); err != nil {
		return err
	}
	instanceID := hello.GetInstanceId()
	if !boundedText(hello.GetBuildVersion(), 256) || !validPlatform(hello.GetPlatform()) || instanceID == "" || !boundedText(instanceID, 64) {
		return nodeapi.Errorf(nodeapi.CodeInvalidArgument, "Hello is malformed")
	}
	node, err := service.database.Node(ctx, hello.GetNodeId())
	if errors.Is(err, store.ErrNodeNotFound) {
		return nodeapi.Errorf(nodeapi.CodeUnauthenticated, "node is not enrolled")
	}
	if err != nil {
		return nodeapi.Errorf(nodeapi.CodeInternal, "read node")
	}
	panelID, err := service.PanelID(ctx)
	if err != nil {
		return nodeapi.Errorf(nodeapi.CodeInternal, "read panel identity")
	}
	nonce := make([]byte, nodeapi.NonceSize)
	if n, err := service.random(nonce); err != nil || n != len(nonce) {
		return nodeapi.Errorf(nodeapi.CodeInternal, "generate challenge")
	}
	if err := stream.Send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Challenge{
		Challenge: &nodeapi.Challenge{PanelId: panelID, Nonce: nonce},
	}}); err != nil {
		return err
	}
	frame, err = receiver.next()
	if err != nil {
		return err
	}
	if !nodeapi.VerifyProof(node.PublicKey, panelID, node.ID, nonce, frame.GetProof()) {
		return nodeapi.Errorf(nodeapi.CodeUnauthenticated, "session proof is invalid")
	}
	sessionID, err := database.NewID(service.random, "session")
	if err != nil {
		return nodeapi.Errorf(nodeapi.CodeInternal, "generate session")
	}
	current := newSession(ctx, cancel, stream)
	current.nodeID, current.instanceID, current.sessionID = node.ID, instanceID, sessionID
	current.protocolMinor, current.buildVersion = hello.GetProtocol().GetMinor(), hello.GetBuildVersion()
	if reported := nodeState(node); reported != nil {
		current.lastUpdate = reported.Report.GetLastUpdate()
	}
	historyAfter, err := service.records.LatestRuntimeTransitionSource(ctx, node.ID, instanceID)
	if err != nil {
		return nodeapi.Errorf(nodeapi.CodeInternal, "read runtime history")
	}
	// Nothing fails between recording the session and replacing the node's
	// previous one, so the record always names the session the registry
	// holds: a later failure ends this session, never a live one.
	err = service.database.RecordNodeSession(ctx, node.ID, node.PublicKey, store.NodeSession{
		ID: sessionID, InstanceID: instanceID, OS: hello.GetPlatform().GetOs(), Arch: hello.GetPlatform().GetArch(), BuildVersion: hello.GetBuildVersion(),
		ProtocolMinor: int(hello.GetProtocol().GetMinor()), SeenAt: service.now().UTC(),
	})
	if errors.Is(err, store.ErrNodeNotFound) {
		return nodeapi.Errorf(nodeapi.CodeUnauthenticated, "node is not enrolled")
	}
	if err != nil {
		return nodeapi.Errorf(nodeapi.CodeInternal, "record session")
	}
	defer func() {
		endCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = service.database.EndNodeSession(endCtx, node.ID, sessionID, service.now().UTC())
		if errors.Is(context.Cause(ctx), errNodeRemoved) {
			// The loop has ended, so no late sample brings the node back.
			service.records.ForgetNode(node.ID)
		}
		service.recordNodeEvent(store.LogLevelInfo, "node.disconnected", "Node disconnected", node.ID)
	}()
	service.sessions.replace(node.ID, current)
	defer service.sessions.remove(node.ID, current)
	// Commands wait for the first state report, which follows Welcome;
	// control frames wait for Welcome itself. The session is registered
	// before Welcome reads the core log policy, so a settings change after
	// that read reaches it once Welcome is sent.
	current.telemetryMu.Lock()
	policy, err := service.coreLogPolicy(ctx)
	if err != nil {
		current.telemetryMu.Unlock()
		return nodeapi.Errorf(nodeapi.CodeInternal, "read settings")
	}
	err = current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Welcome{Welcome: &nodeapi.Welcome{
		SessionId: sessionID, PanelBuildVersion: service.build.Version,
		Protocol:        &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor, Minor: nodeapi.ProtocolMinor},
		MetricsInterval: durationpb.New(MetricsInterval), RuntimeHistoryAfter: historyAfter,
		CoreLogPolicy: policy,
	}}})
	current.welcomed = err == nil
	current.coreLogPolicy = policy
	current.telemetryMu.Unlock()
	if err != nil {
		return err
	}
	service.syncMetricsInterval(node.ID)
	service.recordNodeEvent(store.LogLevelInfo, "node.connected", "Node connected", node.ID)

	validation := time.NewTicker(validationInterval)
	defer validation.Stop()
	// A session that does not report its state in time never accepts
	// commands, so it ends and the node reconnects with a new report.
	firstReport := time.NewTimer(firstReportTimeout)
	defer firstReport.Stop()
	for {
		select {
		case <-ctx.Done():
			return sessionEnd(context.Cause(ctx))
		case err := <-receiver.ended:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case frame := <-receiver.frames:
			if ctx.Err() != nil {
				return sessionEnd(context.Cause(ctx))
			}
			switch {
			case frame.GetResult() != nil:
				current.deliver(frame.GetResult())
			case frame.GetEvent() != nil:
				if err := service.receiveEvent(ctx, current, frame.GetEvent()); err != nil {
					return err
				}
			case frame.GetHello() != nil || frame.GetProof() != nil:
				return nodeapi.Errorf(nodeapi.CodeInvalidArgument, "handshake frames are not allowed after Welcome")
			}
		case <-firstReport.C:
			if !current.isReady() {
				return nodeapi.Errorf(nodeapi.CodeDeadlineExceeded, "the node sent no state report after Welcome")
			}
		case <-validation.C:
			err := service.database.TouchNode(ctx, node.ID, node.PublicKey, service.now().UTC())
			if errors.Is(err, store.ErrNodeNotFound) {
				return sessionEnd(errNodeRemoved)
			}
		}
	}
}

// frameReceiver reads a stream's frames on its own goroutine until the
// stream or ctx ends, so a handler can wait for a frame with a timeout.
type frameReceiver[T any] struct {
	ctx    context.Context
	frames chan T
	// ended receives the error that ended the stream, io.EOF when the
	// node closed it.
	ended chan error
}

func receiveFrames[T any](ctx context.Context, receive func() (T, error)) *frameReceiver[T] {
	receiver := &frameReceiver[T]{ctx: ctx, frames: make(chan T), ended: make(chan error, 1)}
	go func() {
		for {
			frame, err := receive()
			if err != nil {
				receiver.ended <- err
				return
			}
			select {
			case receiver.frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	return receiver
}

// next waits for the next frame of a handshake, at most handshakeTimeout.
func (receiver *frameReceiver[T]) next() (T, error) {
	var none T
	timer := time.NewTimer(handshakeTimeout)
	defer timer.Stop()
	select {
	case frame := <-receiver.frames:
		return frame, nil
	case err := <-receiver.ended:
		return none, err
	case <-timer.C:
		return none, nodeapi.Errorf(nodeapi.CodeDeadlineExceeded, "handshake timed out")
	case <-receiver.ctx.Done():
		return none, contextError(receiver.ctx.Err())
	}
}

// errChunkRead reports a file that failed while sendChunks read it.
var errChunkRead = errors.New("read the file being sent")

// sendChunks reads file in pieces of artifactChunkSize and hands each
// nonempty piece to send, which must encode it before it returns, since the
// buffer is reused.
func sendChunks(file io.Reader, send func(chunk []byte) error) error {
	buffer := make([]byte, artifactChunkSize)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if err := send(buffer[:n]); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %w", errChunkRead, err)
		}
	}
}

// sessionEnd maps the reason the panel ended a session to the status the
// node sees.
func sessionEnd(cause error) error {
	switch {
	case errors.Is(cause, errSessionReplaced):
		return nodeapi.Errorf(nodeapi.CodeAborted, "%s", cause)
	case errors.Is(cause, errNodeRemoved):
		return nodeapi.Errorf(nodeapi.CodeUnauthenticated, "%s", cause)
	case errors.Is(cause, errPanelStopping):
		return nodeapi.Errorf(nodeapi.CodeUnavailable, "%s", cause)
	default:
		return contextError(cause)
	}
}

// contextError maps the error of a context that ended to the status the
// node sees.
func contextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return nodeapi.Errorf(nodeapi.CodeDeadlineExceeded, "%s", err)
	}
	return nodeapi.Errorf(nodeapi.CodeUnavailable, "%s", err)
}

func checkProtocol(version *nodeapi.ProtocolVersion) error {
	if version.GetMajor() != nodeapi.ProtocolMajor {
		return nodeapi.Errorf(nodeapi.CodeFailedPrecondition, "protocol major version %d is not supported; this panel speaks %d", version.GetMajor(), nodeapi.ProtocolMajor)
	}
	return nil
}

func validPlatform(platform *nodeapi.Platform) bool {
	return boundedText(platform.GetOs(), 64) && boundedText(platform.GetArch(), 64)
}

func boundedText(value string, maximum int) bool {
	if len(value) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (service *Service) recordNodeEvent(level store.LogLevel, code, message, nodeID string) {
	if service.records == nil {
		return
	}
	service.records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: level, Code: code, Message: message,
		Metadata: telemetry.LogMetadata(map[string]any{"node_id": nodeID}),
	})
}

// artifactChunkSize is the size of the data in each FetchArtifact frame.
const artifactChunkSize = 64 << 10

// FetchArtifact streams a cached core archive to a node holding a grant for
// it. The first frame carries the artifact's identity and size.
func (server *nodeServer) FetchArtifact(request *nodeapi.FetchArtifactRequest, stream *nodeapi.ArtifactServer) error {
	service := server.service
	nodeID, ok := service.grants.check(request.GetGrant(), request.GetArtifactId(), service.now())
	if !ok {
		return nodeapi.Errorf(nodeapi.CodePermissionDenied, "artifact grant is invalid or expired")
	}
	file, artifact, err := service.cores.OpenCachedArtifact(stream.Context(), request.GetArtifactId())
	if errors.Is(err, store.ErrCachedArtifactNotFound) {
		return nodeapi.Errorf(nodeapi.CodeNotFound, "the artifact is no longer cached")
	}
	if err != nil {
		service.recordNodeEvent(store.LogLevelError, "core.artifact.open_failed", "Cached core archive could not be opened", nodeID)
		return nodeapi.Errorf(nodeapi.CodeInternal, "open cached artifact")
	}
	defer file.Close()
	identity, err := artifactIdentity(artifact)
	if err != nil {
		return nodeapi.Errorf(nodeapi.CodeInternal, "read cached artifact identity")
	}
	// The first frame carries the artifact's identity with the first data.
	frame := &nodeapi.FetchArtifactResponse{Artifact: identity, Size: artifact.Size}
	err = sendChunks(file, func(chunk []byte) error {
		frame.Data = chunk
		sent := stream.Send(frame)
		frame = &nodeapi.FetchArtifactResponse{}
		return sent
	})
	if errors.Is(err, errChunkRead) {
		return nodeapi.Errorf(nodeapi.CodeInternal, "read cached artifact")
	}
	if err == nil && frame.Artifact != nil {
		// An empty archive still names itself.
		err = stream.Send(frame)
	}
	return err
}
