// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"time"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

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
	// MetricsInterval is the baseline interval between metrics samples.
	MetricsInterval = time.Minute
)

// nodeServer serves sbp.node.v1 to nodes.
type nodeServer struct {
	nodeapi.UnimplementedNodeServiceServer
	service *Service
}

func (server *nodeServer) Enroll(ctx context.Context, request *nodeapi.EnrollRequest) (*nodeapi.EnrollResponse, error) {
	service := server.service
	if err := checkProtocol(request.GetProtocol()); err != nil {
		return nil, err
	}
	if len(request.GetPublicKey()) != ed25519.PublicKeySize || !boundedText(request.GetHostname(), 256) ||
		!boundedText(request.GetBuildVersion(), 256) || !validPlatform(request.GetPlatform()) {
		return nil, status.Error(codes.InvalidArgument, "enrollment request is malformed")
	}
	node, err := service.database.EnrollNode(ctx, nodeapi.TokenHash(request.GetToken()), store.NodeEnrollment{
		PublicKey: request.GetPublicKey(), Hostname: request.GetHostname(),
		OS: request.GetPlatform().GetOs(), Arch: request.GetPlatform().GetArch(),
		BuildVersion: request.GetBuildVersion(), ProtocolMinor: int(request.GetProtocol().GetMinor()),
		EnrolledAt: service.now().UTC(),
	})
	if errors.Is(err, store.ErrEnrollmentInvalid) {
		return nil, status.Error(codes.Unauthenticated, "enrollment code is invalid, used or expired")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "record enrollment")
	}
	// A key replaced by enrolling again must not keep its session.
	service.sessions.close(node.ID, errNodeRemoved)
	panelID, err := service.PanelID(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "read panel identity")
	}
	service.recordNodeEvent(store.LogLevelInfo, "node.enrolled", "Node enrolled", node.ID)
	return &nodeapi.EnrollResponse{NodeId: node.ID, PanelId: panelID}, nil
}

// Connect authenticates the node with a signed challenge and then holds its
// session until the stream ends, the session is replaced or the node is
// removed.
func (server *nodeServer) Connect(stream nodeapi.NodeService_ConnectServer) error {
	service := server.service
	ctx, cancel := context.WithCancelCause(stream.Context())
	defer cancel(nil)
	frames := make(chan *nodeapi.ConnectRequest)
	ended := make(chan error, 1)
	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				ended <- err
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	next := func() (*nodeapi.ConnectRequest, error) {
		timer := time.NewTimer(handshakeTimeout)
		defer timer.Stop()
		select {
		case frame := <-frames:
			return frame, nil
		case err := <-ended:
			return nil, err
		case <-timer.C:
			return nil, status.Error(codes.DeadlineExceeded, "handshake timed out")
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}

	frame, err := next()
	if err != nil {
		return err
	}
	hello := frame.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "the session must start with Hello")
	}
	if err := checkProtocol(hello.GetProtocol()); err != nil {
		return err
	}
	if !boundedText(hello.GetBuildVersion(), 256) || !validPlatform(hello.GetPlatform()) {
		return status.Error(codes.InvalidArgument, "Hello is malformed")
	}
	node, err := service.database.Node(ctx, hello.GetNodeId())
	if errors.Is(err, store.ErrNodeNotFound) || (err == nil && node.PublicKey == nil) {
		return status.Error(codes.Unauthenticated, "node is not enrolled")
	}
	if err != nil {
		return status.Error(codes.Internal, "read node")
	}
	panelID, err := service.PanelID(ctx)
	if err != nil {
		return status.Error(codes.Internal, "read panel identity")
	}
	nonce := make([]byte, nodeapi.NonceSize)
	if n, err := service.random(nonce); err != nil || n != len(nonce) {
		return status.Error(codes.Internal, "generate challenge")
	}
	if err := stream.Send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Challenge{
		Challenge: &nodeapi.Challenge{PanelId: panelID, Nonce: nonce},
	}}); err != nil {
		return err
	}
	frame, err = next()
	if err != nil {
		return err
	}
	if !nodeapi.VerifyProof(node.PublicKey, panelID, node.ID, nonce, frame.GetProof()) {
		return status.Error(codes.Unauthenticated, "session proof is invalid")
	}
	sessionID, err := database.NewID(service.random, "session")
	if err != nil {
		return status.Error(codes.Internal, "generate session")
	}
	current := newSession(ctx, cancel, stream)
	service.sessions.replace(node.ID, current)
	defer service.sessions.remove(node.ID, current)
	err = service.database.RecordNodeSession(ctx, node.ID, node.PublicKey, store.NodeSession{
		ID: sessionID, OS: hello.GetPlatform().GetOs(), Arch: hello.GetPlatform().GetArch(), BuildVersion: hello.GetBuildVersion(),
		ProtocolMinor: int(hello.GetProtocol().GetMinor()), SeenAt: service.now().UTC(),
	})
	if errors.Is(err, store.ErrNodeNotFound) {
		return status.Error(codes.Unauthenticated, "node is not enrolled")
	}
	if err != nil {
		return status.Error(codes.Internal, "record session")
	}
	defer func() {
		endCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = service.database.EndNodeSession(endCtx, node.ID, sessionID, service.now().UTC())
		service.recordNodeEvent(store.LogLevelInfo, "node.disconnected", "Node disconnected", node.ID)
	}()
	if err := current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Welcome{Welcome: &nodeapi.Welcome{
		SessionId: sessionID, PanelBuildVersion: service.build.Version,
		Protocol:        &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor, Minor: nodeapi.ProtocolMinor},
		MetricsInterval: durationpb.New(MetricsInterval),
	}}}); err != nil {
		return err
	}
	service.recordNodeEvent(store.LogLevelInfo, "node.connected", "Node connected", node.ID)

	validation := time.NewTicker(validationInterval)
	defer validation.Stop()
	for {
		select {
		case <-ctx.Done():
			return sessionEnd(context.Cause(ctx))
		case err := <-ended:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case frame := <-frames:
			switch {
			case frame.GetResult() != nil:
				current.deliver(frame.GetResult())
			case frame.GetHello() != nil || frame.GetProof() != nil:
				return status.Error(codes.InvalidArgument, "handshake frames are not allowed after Welcome")
			}
		case <-validation.C:
			err := service.database.TouchNode(ctx, node.ID, node.PublicKey, service.now().UTC())
			if errors.Is(err, store.ErrNodeNotFound) {
				return sessionEnd(errNodeRemoved)
			}
		}
	}
}

// sessionEnd maps the reason the panel ended a session to the status the
// node sees.
func sessionEnd(cause error) error {
	switch {
	case errors.Is(cause, errSessionReplaced):
		return status.Error(codes.Aborted, cause.Error())
	case errors.Is(cause, errNodeRemoved):
		return status.Error(codes.Unauthenticated, cause.Error())
	case errors.Is(cause, errPanelStopping):
		return status.Error(codes.Unavailable, cause.Error())
	default:
		return status.FromContextError(cause).Err()
	}
}

func checkProtocol(version *nodeapi.ProtocolVersion) error {
	if version.GetMajor() != nodeapi.ProtocolMajor {
		return status.Errorf(codes.FailedPrecondition, "protocol major version %d is not supported; this panel speaks %d", version.GetMajor(), nodeapi.ProtocolMajor)
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
func (server *nodeServer) FetchArtifact(request *nodeapi.FetchArtifactRequest, stream nodeapi.NodeService_FetchArtifactServer) error {
	service := server.service
	nodeID, ok := service.grants.check(request.GetGrant(), request.GetArtifactId(), service.now())
	if !ok {
		return status.Error(codes.PermissionDenied, "artifact grant is invalid or expired")
	}
	file, artifact, err := service.cores.OpenCachedArtifact(stream.Context(), request.GetArtifactId())
	if errors.Is(err, store.ErrCachedArtifactNotFound) {
		return status.Error(codes.NotFound, "the artifact is no longer cached")
	}
	if err != nil {
		service.recordNodeEvent(store.LogLevelError, "core.artifact.open_failed", "Cached core archive could not be opened", nodeID)
		return status.Error(codes.Internal, "open cached artifact")
	}
	defer file.Close()
	identity, err := artifactIdentity(artifact)
	if err != nil {
		return status.Error(codes.Internal, "read cached artifact identity")
	}
	frame := &nodeapi.FetchArtifactResponse{Artifact: identity, Size: artifact.Size}
	buffer := make([]byte, artifactChunkSize)
	for {
		n, err := file.Read(buffer)
		if n > 0 || frame.Artifact != nil {
			// Send encodes the frame before it returns, so buffer can be reused.
			frame.Data = buffer[:n]
			if err := stream.Send(frame); err != nil {
				return err
			}
			frame = &nodeapi.FetchArtifactResponse{}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return status.Error(codes.Internal, "read cached artifact")
		}
	}
}
