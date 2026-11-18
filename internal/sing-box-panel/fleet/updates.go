// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

const (
	// UpdateTimeout bounds an update command, which includes the transfer.
	UpdateTimeout = 10 * time.Minute
	// UpdateReturnTimeout bounds how long a batch update waits for an
	// updated node to come back reporting the panel's version.
	UpdateReturnTimeout = 5 * time.Minute
)

// Node update states.
const (
	UpdateUpToDate        = "up_to_date"
	UpdateRestarting      = "restarting"
	UpdateStaged          = "staged"
	UpdateRestartRequired = "restart_required"
)

var (
	// ErrNodeUpgradeRequired rejects a command for a node whose protocol
	// minor version predates it.
	ErrNodeUpgradeRequired = errors.New("the node's protocol version is too old for this command; update the node with the installer")
	// ErrNodeRestartRequired reports a node that installed the update but
	// runs it only once restarted by hand.
	ErrNodeRestartRequired = errors.New("the node installed the update and runs it once it is restarted")
	// ErrNodeUpdateTimeout reports an updated node that did not come back
	// reporting the new version in time.
	ErrNodeUpdateTimeout = errors.New("the node did not come back with the new version in time")
	// ErrSkipped reports a node a sequential batch did not reach because an
	// earlier node failed.
	ErrSkipped = errors.New("skipped after an earlier node failed")
)

// NodeUpdateFailure is a node's report that its update failed, such as its
// root update helper's.
type NodeUpdateFailure struct {
	Version string
	Code    string
}

func (failure *NodeUpdateFailure) Error() string {
	return "the node's update to " + failure.Version + " failed: " + failure.Code
}

// commandMinor is the lowest protocol minor version that has each command;
// commands it does not name are in minor version 0.
func commandMinor(body nodeapi.CommandBody) uint32 {
	switch body.(type) {
	case *nodeapi.Command_UpdateNode:
		return 1
	}
	return 0
}

// NodeUpdate is how an update command left a node.
type NodeUpdate struct {
	NodeID  string `json:"node_id"`
	Version string `json:"version"`
	// State is up_to_date, restarting, staged or restart_required.
	State string `json:"state"`
}

// UpdateAttempt is a node's last update attempt as it reported it.
type UpdateAttempt struct {
	Version    string     `json:"version"`
	Succeeded  bool       `json:"succeeded"`
	Code       string     `json:"code"`
	FinishedAt *time.Time `json:"finished_at"`
}

func updateAttemptView(attempt *nodeapi.UpdateAttempt) *UpdateAttempt {
	if attempt == nil {
		return nil
	}
	view := &UpdateAttempt{Version: attempt.GetVersion(), Succeeded: attempt.GetSucceeded(), Code: attempt.GetCode()}
	if attempt.GetFinishedAt() != nil {
		finished := attempt.GetFinishedAt().AsTime()
		view.FinishedAt = &finished
	}
	return view
}

// releaseVersion is the node release the panel distributes, empty when it
// has none.
func (service *Service) releaseVersion() string {
	if service.releases == nil {
		return ""
	}
	version, err := service.releases.NodeVersion()
	if err != nil {
		return ""
	}
	return version
}

// UpdateNode asks the node to update itself to the panel's release and
// returns how the node left it.
func (service *Service) UpdateNode(ctx context.Context, nodeID string) (result NodeUpdate, err error) {
	defer service.recordCommand(ctx, "node.update", "Node update", nodeID, &err)
	return service.updateNode(ctx, nodeID)
}

func (service *Service) updateNode(ctx context.Context, nodeID string) (NodeUpdate, error) {
	if service.releases == nil {
		return NodeUpdate{}, releases.ErrUnavailable
	}
	version, err := service.releases.NodeVersion()
	if err != nil {
		return NodeUpdate{}, err
	}
	record, err := service.database.Node(ctx, nodeID)
	if err != nil {
		return NodeUpdate{}, err
	}
	if current := service.sessions.get(nodeID); current != nil && current.protocolMinor < commandMinor(&nodeapi.Command_UpdateNode{}) {
		return NodeUpdate{}, ErrNodeUpgradeRequired
	}
	// The node fetches from a cache the panel has verified.
	if _, err := service.releases.NodeRelease(ctx, record.OS, record.Arch); err != nil {
		return NodeUpdate{}, releases.Failed(err)
	}
	ctx, cancel := context.WithTimeout(ctx, UpdateTimeout)
	defer cancel()
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_UpdateNode{UpdateNode: &nodeapi.UpdateNode{Version: version}})
	if err != nil {
		return NodeUpdate{}, err
	}
	update := outcome.GetUpdate()
	state := ""
	switch update.GetState() {
	case nodeapi.UpdateState_UPDATE_STATE_UP_TO_DATE:
		state = UpdateUpToDate
	case nodeapi.UpdateState_UPDATE_STATE_RESTARTING:
		state = UpdateRestarting
	case nodeapi.UpdateState_UPDATE_STATE_STAGED:
		state = UpdateStaged
	case nodeapi.UpdateState_UPDATE_STATE_RESTART_REQUIRED:
		state = UpdateRestartRequired
	}
	if update == nil || state == "" || update.GetVersion() != version {
		return NodeUpdate{}, errUnexpectedResult
	}
	return NodeUpdate{NodeID: nodeID, Version: version, State: state}, nil
}

// UpdateNodeAndWait updates the node and, unless it already runs the
// panel's release, waits until it comes back reporting it, also when its
// session ended before its result arrived. A node that
// reports a failed attempt at the release other than the one it reported
// before the update fails with that attempt's code; one that must be
// restarted by hand fails with ErrNodeRestartRequired.
func (service *Service) UpdateNodeAndWait(ctx context.Context, nodeID string) (err error) {
	defer service.recordCommand(ctx, "node.update", "Node update", nodeID, &err)
	// The attempt the node reports once its session is ready, before the
	// command is sent, is not this update's outcome.
	if current := service.sessions.get(nodeID); current != nil {
		select {
		case <-current.ready:
		case <-current.done:
		case <-ctx.Done():
		}
	}
	before := service.reportedAttempt(ctx, nodeID)
	result, err := service.updateNode(ctx, nodeID)
	if errors.Is(err, ErrNodeResultUnknown) {
		// A node restarting into the update may end its session before its
		// result arrives; the version it comes back with settles it.
		version, versionErr := service.releases.NodeVersion()
		if versionErr != nil {
			return err
		}
		return service.waitForVersion(ctx, nodeID, version, before)
	}
	if err != nil {
		return err
	}
	switch result.State {
	case UpdateUpToDate:
		return nil
	case UpdateRestartRequired:
		return ErrNodeRestartRequired
	}
	return service.waitForVersion(ctx, nodeID, result.Version, before)
}

// reportedAttempt is the update attempt the node last reported, if any.
func (service *Service) reportedAttempt(ctx context.Context, nodeID string) *nodeapi.UpdateAttempt {
	record, err := service.database.Node(ctx, nodeID)
	if err != nil {
		return nil
	}
	if state := nodeState(record); state != nil {
		return state.Report.GetLastUpdate()
	}
	return nil
}

func (service *Service) waitForVersion(ctx context.Context, nodeID, version string, before *nodeapi.UpdateAttempt) error {
	ctx, cancel := context.WithTimeout(ctx, UpdateReturnTimeout)
	defer cancel()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		if current := service.sessions.get(nodeID); current != nil && current.isReady() {
			if current.buildVersion == version {
				return nil
			}
			if failure := service.failedAttempt(ctx, nodeID, version, before); failure != nil {
				return failure
			}
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrNodeUpdateTimeout
			}
			return ctx.Err()
		case <-poll.C:
		}
	}
}

// failedAttempt returns the failure the node reports for version, if its
// last attempt failed and is not the attempt it reported before the update,
// told apart by when each finished.
func (service *Service) failedAttempt(ctx context.Context, nodeID, version string, before *nodeapi.UpdateAttempt) error {
	last := service.reportedAttempt(ctx, nodeID)
	if last == nil || last.GetSucceeded() || last.GetVersion() != version {
		return nil
	}
	if before != nil && last.GetFinishedAt().AsTime().Equal(before.GetFinishedAt().AsTime()) {
		return nil
	}
	return &NodeUpdateFailure{Version: version, Code: last.GetCode()}
}

// RunSequential runs command on each node one at a time, in order, and
// stops at the first failure: an ID of no node fails with
// store.ErrNodeNotFound, and every node after a failure is reported with
// ErrSkipped. Like RunBatch it keeps running when ctx is cancelled and
// calls report once per node.
func (service *Service) RunSequential(ctx context.Context, nodes []BatchNode, command func(context.Context, Node) error, report func(BatchResult)) {
	ctx = context.WithoutCancel(ctx)
	failed := false
	for _, node := range nodes {
		var err error
		switch {
		case failed:
			err = ErrSkipped
		case node.Node == nil:
			err = store.ErrNodeNotFound
		default:
			err = command(ctx, *node.Node)
		}
		failed = failed || err != nil
		report(BatchResult{NodeID: node.ID, Err: err})
	}
}

// recordUpdateReport logs an update attempt a node reports for the first
// time, such as its root update helper's.
func (service *Service) recordUpdateReport(nodeID string, attempt *nodeapi.UpdateAttempt) {
	if service.records == nil {
		return
	}
	level, code, message := store.LogLevelInfo, "node.update.applied", "The node installed a new version"
	if !attempt.GetSucceeded() {
		level, code, message = store.LogLevelError, "node.update.rejected", "The node could not install a new version"
	}
	service.records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: level, Code: code, Message: message,
		Metadata: telemetry.LogMetadata(map[string]any{"node_id": nodeID, "version": attempt.GetVersion(), "error_code": attempt.GetCode()}),
	})
}

// FetchRelease authenticates the node with its own signed challenge, apart
// from its session, and streams the panel's node release for the node's
// platform: the header first, then the binary.
func (server *nodeServer) FetchRelease(stream nodeapi.NodeService_FetchReleaseServer) error {
	service := server.service
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	frames := make(chan *nodeapi.FetchReleaseRequest, 1)
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
	next := func() (*nodeapi.FetchReleaseRequest, error) {
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
		return status.Error(codes.InvalidArgument, "FetchRelease must start with Hello")
	}
	if err := checkProtocol(hello.GetProtocol()); err != nil {
		return err
	}
	if !validPlatform(hello.GetPlatform()) {
		return status.Error(codes.InvalidArgument, "Hello is malformed")
	}
	node, err := service.database.Node(ctx, hello.GetNodeId())
	if errors.Is(err, store.ErrNodeNotFound) {
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
	if err := stream.Send(&nodeapi.FetchReleaseResponse{Frame: &nodeapi.FetchReleaseResponse_Challenge{
		Challenge: &nodeapi.Challenge{PanelId: panelID, Nonce: nonce},
	}}); err != nil {
		return err
	}
	if frame, err = next(); err != nil {
		return err
	}
	if !nodeapi.VerifyReleaseProof(node.PublicKey, panelID, node.ID, nonce, frame.GetProof()) {
		return status.Error(codes.Unauthenticated, "release proof is invalid")
	}
	if service.releases == nil {
		return status.Error(codes.FailedPrecondition, releases.ErrUnavailable.Error())
	}
	release, err := service.releases.NodeRelease(ctx, hello.GetPlatform().GetOs(), hello.GetPlatform().GetArch())
	switch {
	case errors.Is(err, releases.ErrUnavailable), errors.Is(err, releases.ErrPlatformUnsupported):
		return status.Error(codes.FailedPrecondition, err.Error())
	case err != nil:
		service.recordNodeEvent(store.LogLevelError, "node.release.unavailable", "The node release could not be prepared", node.ID)
		return status.Error(codes.Unavailable, "the panel could not prepare its node release")
	}
	file, err := os.Open(release.Path)
	if err != nil {
		return status.Error(codes.Internal, "open the node release")
	}
	defer file.Close()
	if err := stream.Send(&nodeapi.FetchReleaseResponse{Frame: &nodeapi.FetchReleaseResponse_Header{Header: &nodeapi.ReleaseHeader{
		Version: release.Version, Size: release.Size, Sha256: release.SHA256, Checksums: release.Checksums, Signature: release.Signature,
	}}}); err != nil {
		return err
	}
	buffer := make([]byte, artifactChunkSize)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			// Send encodes the frame before it returns, so buffer can be reused.
			if err := stream.Send(&nodeapi.FetchReleaseResponse{Frame: &nodeapi.FetchReleaseResponse_Chunk{Chunk: buffer[:n]}}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return status.Error(codes.Internal, "read the node release")
		}
	}
}
