// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
)

// enrolledRawNode adds a node and enrolls a raw node speaking minor version
// minor as build version build.
func enrolledRawNode(t *testing.T, served testFleet, name string, minor uint32, build string) *rawNode {
	t.Helper()
	enrollment, err := served.service.AddNode(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.url), minor: minor, build: build}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	return node
}

// fetchRelease runs FetchRelease as the raw node, answering the challenge
// with sign, and returns the header and the binary.
func (node *rawNode) fetchRelease(t *testing.T, sign func(panelID string, nonce []byte) *nodeapi.Proof) (*nodeapi.ReleaseHeader, []byte, error) {
	t.Helper()
	stream, err := node.client.FetchRelease(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&nodeapi.FetchReleaseRequest{Frame: &nodeapi.FetchReleaseRequest_Hello{Hello: &nodeapi.Hello{
		NodeId: node.id, Protocol: &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor, Minor: 1}, Platform: &nodeapi.Platform{Os: "linux", Arch: "amd64"},
	}}}); err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil {
		return nil, nil, err
	}
	challenge := frame.GetChallenge()
	if err := stream.Send(&nodeapi.FetchReleaseRequest{Frame: &nodeapi.FetchReleaseRequest_Proof{
		Proof: sign(challenge.GetPanelId(), challenge.GetNonce()),
	}}); err != nil {
		t.Fatal(err)
	}
	frame, err = stream.Recv()
	if err != nil {
		return nil, nil, err
	}
	var binary []byte
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return frame.GetHeader(), binary, nil
		}
		if err != nil {
			return nil, nil, err
		}
		binary = append(binary, chunk.GetChunk()...)
	}
}

func TestFetchReleaseNeedsItsOwnProof(t *testing.T) {
	// Larger than one frame.
	binary := bytes.Repeat([]byte("node binary "), 10_000)
	served := startFleetWith(t, fleetSetup{version: "v1.1.0", release: map[string][]byte{"amd64": binary}})
	node := enrolledRawNode(t, served, "edge", 1, "v1.0.0")

	_, _, err := node.fetchRelease(t, func(panelID string, nonce []byte) *nodeapi.Proof {
		return nodeapi.SignProof(node.key, panelID, node.id, nonce)
	})
	requireCode(t, err, nodeapi.CodeUnauthenticated)
	other := enrolledRawNode(t, served, "other", 1, "v1.0.0")
	_, _, err = node.fetchRelease(t, func(panelID string, nonce []byte) *nodeapi.Proof {
		return nodeapi.SignReleaseProof(other.key, panelID, node.id, nonce)
	})
	requireCode(t, err, nodeapi.CodeUnauthenticated)

	header, received, err := node.fetchRelease(t, func(panelID string, nonce []byte) *nodeapi.Proof {
		return nodeapi.SignReleaseProof(node.key, panelID, node.id, nonce)
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	if header.GetVersion() != "v1.1.0" || header.GetSize() != int64(len(binary)) || header.GetSha256() != hex.EncodeToString(digest[:]) ||
		!bytes.Equal(received, binary) || len(header.GetChecksums()) == 0 || len(header.GetSignature()) == 0 {
		t.Fatalf("header %+v with %d bytes; want the cached amd64 release", header, len(received))
	}
}

func TestUpdateNeedsTheProtocolMinorThatHasIt(t *testing.T) {
	served := startFleetWith(t, fleetSetup{version: "v1.1.0", release: map[string][]byte{}})
	old := enrolledRawNode(t, served, "old", 0, "v1.0.0")
	stream, err := old.connect(t, old.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, served.service, old.id, fleet.StateOnline)
	if _, err := served.service.UpdateNode(t.Context(), old.id); !errors.Is(err, fleet.ErrNodeUpgradeRequired) {
		t.Fatalf("update of a minor 0 node = %v, want ErrNodeUpgradeRequired", err)
	}
	if _, err := served.service.CoreSchema(t.Context(), old.id, "core"); !errors.Is(err, fleet.ErrNodeUpgradeRequired) {
		t.Fatalf("core schema of a minor 0 node = %v, want ErrNodeUpgradeRequired", err)
	}
	// Commands of minor version 0 still reach it.
	stopped := make(chan error, 1)
	go func() {
		_, err := served.service.StopRuntime(t.Context(), old.id)
		stopped <- err
	}()
	frame, err := stream.Recv()
	if err != nil || frame.GetCommand().GetStopRuntime() == nil {
		t.Fatalf("expected a stop command first, got %v, %v", frame, err)
	}
	answerCommand(t, stream, frame.GetCommand().GetRequestId(), &nodeapi.Result_Runtime{Runtime: &nodeapi.RuntimeOutcome{Status: &nodeapi.RuntimeStatus{}}})
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}

	current := enrolledRawNode(t, served, "current", 1, "v1.1.0")
	stream, err = current.connect(t, current.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, served.service, current.id, fleet.StateOnline)
	go func() {
		frame, err := stream.Recv()
		if err == nil && frame.GetCommand().GetUpdateNode().GetVersion() == "v1.1.0" {
			answerCommand(t, stream, frame.GetCommand().GetRequestId(), &nodeapi.Result_Update{Update: &nodeapi.UpdateOutcome{
				State: nodeapi.UpdateState_UPDATE_STATE_UP_TO_DATE, Version: "v1.1.0",
			}})
		}
	}()
	result, err := served.service.UpdateNode(t.Context(), current.id)
	if err != nil || result.State != fleet.UpdateUpToDate || result.Version != "v1.1.0" {
		t.Fatalf("update = %+v, %v; want up to date", result, err)
	}
}

func answerCommand(t *testing.T, stream *nodeapi.ConnectClient, requestID string, outcome nodeapi.ResultOutcome) {
	t.Helper()
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: &nodeapi.Result{
		RequestId: requestID, Outcome: outcome,
	}}}); err != nil {
		t.Error(err)
	}
}

func TestBatchUpdateRunsOneNodeAtATimeAndStopsAtAFailure(t *testing.T) {
	served := startFleetWith(t, fleetSetup{version: "v1.1.0", release: map[string][]byte{}})
	var nodes []*rawNode
	var streams []*nodeapi.ConnectClient
	for _, name := range []string{"first", "second", "third"} {
		node := enrolledRawNode(t, served, name, 1, "v1.0.0")
		stream, err := node.connect(t, node.key)
		if err != nil {
			t.Fatal(err)
		}
		waitState(t, served.service, node.id, fleet.StateOnline)
		nodes, streams = append(nodes, node), append(streams, stream)
	}
	if listed, err := served.service.Node(t.Context(), nodes[0].id); err != nil || !listed.UpdateAvailable {
		t.Fatalf("a node behind the panel's release = %+v, %v; want an update available", listed, err)
	}
	order := make(chan string, 8)
	// The first node restarts into the release and comes back with it.
	go func() {
		frame, err := streams[0].Recv()
		if err != nil {
			t.Error(err)
			return
		}
		order <- "first"
		answerCommand(t, streams[0], frame.GetCommand().GetRequestId(), &nodeapi.Result_Update{Update: &nodeapi.UpdateOutcome{
			State: nodeapi.UpdateState_UPDATE_STATE_RESTARTING, Version: "v1.1.0",
		}})
		_ = streams[0].Close(nil)
		time.Sleep(200 * time.Millisecond)
		nodes[0].build = "v1.1.0"
		if _, err := nodes[0].connect(t, nodes[0].key); err != nil {
			t.Error(err)
		}
	}()
	// The second node cannot verify the release.
	go func() {
		frame, err := streams[1].Recv()
		if err != nil {
			return
		}
		order <- "second"
		answerCommand(t, streams[1], frame.GetCommand().GetRequestId(), &nodeapi.Result_Problem{Problem: &nodeapi.Problem{
			Code: "update_signature_invalid", Title: "Node command failed", Detail: "release signature is invalid",
		}})
	}()
	go func() {
		if frame, err := streams[2].Recv(); err == nil {
			t.Errorf("the node after a failure received %v", frame)
		}
	}()
	targets, err := served.service.BatchNodes(t.Context(), fleet.BatchTarget{NodeIDs: []string{nodes[0].id, nodes[1].id, nodes[2].id}})
	if err != nil {
		t.Fatal(err)
	}
	var results []fleet.BatchResult
	served.service.RunSequential(t.Context(), targets, func(ctx context.Context, node fleet.Node) error {
		return served.service.UpdateNodeAndWait(ctx, node.ID)
	}, func(result fleet.BatchResult) { results = append(results, result) })
	if len(results) != 3 || results[0].Err != nil || !fleet.IsProblem(results[1].Err, "update_signature_invalid") || !errors.Is(results[2].Err, fleet.ErrSkipped) {
		t.Fatalf("results = %+v; want the first updated, the second failed and the third skipped", results)
	}
	if first, second := <-order, <-order; first != "first" || second != "second" {
		t.Fatalf("commanded %s before %s", first, second)
	}
	if updated, err := served.service.Node(t.Context(), nodes[0].id); err != nil || updated.BuildVersion != "v1.1.0" || updated.UpdateAvailable {
		t.Fatalf("updated node = %+v, %v", updated, err)
	}
}

// TestRetriedUpdateIsNotBlamedForTheLastFailure retries an update soon after
// the node's helper refused one: until the node comes back, it still reports
// that earlier failure, which is not this update's outcome.
func TestRetriedUpdateIsNotBlamedForTheLastFailure(t *testing.T) {
	served := startFleetWith(t, fleetSetup{version: "v1.1.0", release: map[string][]byte{}})
	node := enrolledRawNode(t, served, "edge", 1, "v1.0.0")
	stream, _, err := node.handshake(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	earlier := &nodeapi.UpdateAttempt{Version: "v1.1.0", Code: "signature_invalid", FinishedAt: timestamppb.New(time.Now().Add(-10 * time.Second))}
	sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: &nodeapi.StateReport{LastUpdate: earlier}}})
	waitState(t, served.service, node.id, fleet.StateOnline)
	done := make(chan error, 1)
	go func() { done <- served.service.UpdateNodeAndWait(t.Context(), node.id) }()
	frame, err := stream.Recv()
	if err != nil || frame.GetCommand().GetUpdateNode() == nil {
		t.Fatalf("expected an update command, got %v, %v", frame, err)
	}
	answerCommand(t, stream, frame.GetCommand().GetRequestId(), &nodeapi.Result_Update{Update: &nodeapi.UpdateOutcome{
		State: nodeapi.UpdateState_UPDATE_STATE_STAGED, Version: "v1.1.0",
	}})
	// The helper applies the retry while the node still reports the failure.
	select {
	case err := <-done:
		t.Fatalf("the update ended with %v before the node came back", err)
	case <-time.After(1500 * time.Millisecond):
	}
	_ = stream.Close(nil)
	node.build = "v1.1.0"
	if _, err := node.connect(t, node.key); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the retried update = %v, want success", err)
	}
}

// TestBatchUpdateWaitsForANodeThatRestartedBeforeAnswering ends the node's
// session before its result arrives, as a restart into the update can: the
// node coming back with the panel's version settles the update.
func TestBatchUpdateWaitsForANodeThatRestartedBeforeAnswering(t *testing.T) {
	served := startFleetWith(t, fleetSetup{version: "v1.1.0", release: map[string][]byte{}})
	node := enrolledRawNode(t, served, "edge", 1, "v1.0.0")
	stream, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, served.service, node.id, fleet.StateOnline)
	done := make(chan error, 1)
	go func() { done <- served.service.UpdateNodeAndWait(t.Context(), node.id) }()
	if frame, err := stream.Recv(); err != nil || frame.GetCommand().GetUpdateNode() == nil {
		t.Fatalf("expected an update command, got %v, %v", frame, err)
	}
	_ = stream.Close(nil)
	waitState(t, served.service, node.id, fleet.StateOffline)
	node.build = "v1.1.0"
	if _, err := node.connect(t, node.key); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("an update whose result was lost to the restart = %v, want success", err)
	}
}

// TestNodeUpdatesItselfFromThePanelsRelease runs a real node, which fetches
// the panel's cached release, verifies it with its own key and installs it.
func TestNodeUpdatesItselfFromThePanelsRelease(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("the node runs its core runtime only on Linux")
	}
	// The release must be a real sing-box-node build for this platform.
	built := filepath.Join(t.TempDir(), "sing-box-node")
	command := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-buildvcs=false", "-o", built, "github.com/rehuony/sing-box-panel/cmd/sing-box-node")
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build the node: %v\n%s", err, output)
	}
	binary, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SING_BOX_SYSTEMD_SCOPE", "")
	served := startFleetWith(t, fleetSetup{version: "v1.1.0", release: map[string][]byte{goruntime.GOARCH: binary}})
	enrollment, err := served.service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	nodeDir := t.TempDir()
	build := buildinfo.Info{Version: "v1.0.0"}
	if _, err := agent.Enroll(t.Context(), nodeDir, enrollment.Code, build); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "sing-box-node")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	link, err := agent.OpenLink(nodeDir, build)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx, link, agent.Options{Update: agent.UpdateOptions{
			PublicKey: served.releaseKey, Executable: func() (string, error) { return executable, nil },
		}})
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	}()
	waitState(t, served.service, enrollment.NodeID, fleet.StateOnline)

	result, err := served.service.UpdateNode(t.Context(), enrollment.NodeID)
	if err != nil || result.State != fleet.UpdateRestartRequired || result.Version != "v1.1.0" {
		t.Fatalf("update = %+v, %v; want installed, waiting for a restart", result, err)
	}
	if installed, err := os.ReadFile(executable); err != nil || !bytes.Equal(installed, binary) {
		t.Fatalf("the executable is not the release: %v", err)
	}
	if previous, err := os.ReadFile(executable + selfupdate.PreviousSuffix); err != nil || string(previous) != "old binary" {
		t.Fatalf("previous executable = %q, %v", previous, err)
	}
	node, err := served.service.Node(t.Context(), enrollment.NodeID)
	if err != nil || node.LastUpdate == nil || !node.LastUpdate.Succeeded || node.LastUpdate.Version != "v1.1.0" || !node.UpdateAvailable {
		t.Fatalf("node after its update = %+v, %v; want the attempt reported and v1.0.0 still running", node, err)
	}
}
