// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/release"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (value *clock) Now() time.Time {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.now
}

func (value *clock) Advance(by time.Duration) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.now = value.now.Add(by)
}

const basePath = "/panel"

// testFleet is a served fleet and the cache its nodes install from.
type testFleet struct {
	service  *fleet.Service
	configs  *nodeconfig.Service
	cores    *corecache.Service
	records  *telemetry.Service
	database *store.Store
	endpoint string
	dataDir  string
	// releaseKey verifies the node release in the panel's cache.
	releaseKey ed25519.PublicKey
}

// fleetSetup adjusts a served fleet: the panel's clock, its version, and a
// signed node release of that version in its cache, by architecture.
type fleetSetup struct {
	now     func() time.Time
	version string
	release map[string][]byte
}

// startFleet serves a fleet and a stand-in Web server on one loopback port,
// with the node service under basePath.
func startFleet(t *testing.T, now func() time.Time) testFleet {
	t.Helper()
	return startFleetWith(t, fleetSetup{now: now})
}

// cacheRelease writes a node release of version signed with a new key into
// the panel's release cache and returns the key that verifies it.
func cacheRelease(t *testing.T, dataDir, version string, binaries map[string][]byte) ed25519.PublicKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(dataDir, releases.CacheDir, version)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var checksums strings.Builder
	for _, arch := range []string{"amd64", "arm64"} {
		name := selfupdate.Node.AssetName("linux", arch)
		binary := binaries[arch]
		if binary == nil {
			binary = []byte("node binary for " + arch)
		}
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(binary), name)
		if err := os.WriteFile(filepath.Join(dir, name), binary, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	signature, err := release.Sign(private, version, []byte(checksums.String()))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{selfupdate.ChecksumsName: []byte(checksums.String()), selfupdate.SignatureName: signature} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return public
}

func startFleetWith(t *testing.T, setup fleetSetup) testFleet {
	t.Helper()
	now := setup.now
	if setup.version == "" {
		setup.version = "test"
	}
	dataDir := t.TempDir()
	database, err := store.Open(t.Context(), filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	value := settings.Defaults()
	value.DataDir = dataDir
	value.Server.BasePath = basePath
	value.Nodes.Endpoint = listener.Addr().String()
	panelSettings := settings.NewService(database, "", value)
	records := telemetry.New(database, panelSettings, telemetry.Options{})
	cores := corecache.New(database, records, panelSettings, corecache.Options{DataDir: dataDir})
	configs := nodeconfig.New(database, cores)
	var releaseKey ed25519.PublicKey
	if setup.release != nil {
		releaseKey = cacheRelease(t, dataDir, setup.version, setup.release)
	}
	build := buildinfo.Info{Version: setup.version}
	service := fleet.New(database, panelSettings, records, configs, cores, fleet.Options{
		Build: build, Now: now,
		Releases: releases.New(build, dataDir, panelSettings, records, releases.Options{PublicKey: releaseKey}),
	})
	web, nodes := fleet.SplitListener(listener)
	webServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "web") })}
	go func() { _ = webServer.Serve(web) }()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- service.Serve(ctx, nodes) }()
	t.Cleanup(func() {
		_ = webServer.Close()
		cancel()
		if err := <-served; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	return testFleet{
		service: service, configs: configs, cores: cores, records: records, database: database,
		endpoint: listener.Addr().String(), dataDir: dataDir, releaseKey: releaseKey,
	}
}

func waitState(t *testing.T, service *fleet.Service, id, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		node, err := service.Node(t.Context(), id)
		if err == nil && node.State == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("node state = %q, %v; want %q", node.State, err, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNodeSharesTheWebPortAndLosesSessionWhenRemoved(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("the node runs its core runtime only on Linux")
	}
	served := startFleet(t, nil)
	service, endpoint := served.service, served.endpoint
	response, err := http.Get("http://" + endpoint + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "web" {
		t.Fatalf("Web request reached %q", body)
	}
	enrollment, err := service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enrollment.Command, "sing-box-node enroll --code ") {
		t.Fatalf("enrollment = %+v", enrollment)
	}
	nodeDir := t.TempDir()
	build := buildinfo.Info{Version: "test"}
	enrolled, err := agent.Enroll(t.Context(), nodeDir, enrollment.Code, build)
	if err != nil {
		t.Fatal(err)
	}
	if enrolled.NodeID != enrollment.NodeID || enrolled.Path != basePath || enrolled.Pin == "" {
		t.Fatalf("enrolled as %+v, want node %q under %s with a pin", enrolled, enrollment.NodeID, basePath)
	}
	var rejected *agent.EnrollmentError
	if _, err := agent.Enroll(t.Context(), t.TempDir(), enrollment.Code, build); !errors.As(err, &rejected) || !rejected.Rejected {
		t.Fatalf("a used enrollment code: %v, want a rejection", err)
	}

	link, err := agent.OpenLink(nodeDir, build)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	logs := make(chan string, 256)
	connected := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx, link, agent.Options{
			Logf: func(format string, args ...any) {
				select {
				case logs <- format:
				default:
				}
			},
			Connected: func() {
				select {
				case connected <- struct{}{}:
				default:
				}
			},
		})
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	}()
	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("the node did not connect")
	}
	waitState(t, service, enrolled.NodeID, fleet.StateOnline)
	// The node reports its state as soon as its session opens.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		node, err := service.Node(t.Context(), enrolled.NodeID)
		if err == nil && node.ReportedAt != nil && node.Runtime != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the node did not report its state: %+v, %v", node, err)
		}
	}

	if err := service.DeleteNode(t.Context(), enrolled.NodeID); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case line := <-logs:
			if strings.HasPrefix(line, "session with panel") {
				reenrollAndUnenroll(t, service, link, connected)
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("removing the node did not end its session")
		}
	}
}

// reenrollAndUnenroll enrolls a running node as another node, as its console
// does, and then removes its enrollment.
func reenrollAndUnenroll(t *testing.T, service *fleet.Service, link *agent.Link, connected <-chan struct{}) {
	t.Helper()
	enrollment, err := service.AddNode(t.Context(), "edge-2")
	if err != nil {
		t.Fatal(err)
	}
	status, err := link.Enroll(t.Context(), enrollment.Code)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enrollment == nil || status.Enrollment.NodeID != enrollment.NodeID || status.State != agent.ConnectionConnecting {
		t.Fatalf("status after enrolling = %+v", status)
	}
	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("the re-enrolled node did not connect")
	}
	waitState(t, service, enrollment.NodeID, fleet.StateOnline)
	if status := link.Status(); status.State != agent.ConnectionConnected {
		t.Fatalf("status after connecting = %+v", status)
	}
	if status, err = link.Unenroll(); err != nil || status.State != agent.ConnectionUnenrolled || status.Enrollment != nil {
		t.Fatalf("unenroll = %+v, %v", status, err)
	}
	waitState(t, service, enrollment.NodeID, fleet.StateOffline)
}

// rawNode speaks the protocol directly to exercise the panel's checks. Its
// Hello names protocol minor version minor and build version build.
type rawNode struct {
	client nodeapi.NodeServiceClient
	key    ed25519.PrivateKey
	id     string
	minor  uint32
	build  string
}

// dialRaw connects over TLS, or over cleartext HTTP/2 as a reverse proxy on
// this host would, sending every method under prefix.
func dialRaw(t *testing.T, endpoint, prefix string, secure bool) nodeapi.NodeServiceClient {
	t.Helper()
	transport := insecure.NewCredentials()
	if secure {
		transport = credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
	}
	connection, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(transport),
		grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, request, reply any, connection *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
			return invoke(ctx, prefix+method, request, reply, connection, options...)
		}),
		grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, connection *grpc.ClientConn, method string, stream grpc.Streamer, options ...grpc.CallOption) (grpc.ClientStream, error) {
			return stream(ctx, desc, connection, prefix+method, options...)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return nodeapi.NewNodeServiceClient(connection)
}

func (node *rawNode) enroll(t *testing.T, code string) error {
	t.Helper()
	parsed, err := nodeapi.ParseEnrollmentCode(code)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	response, err := node.client.Enroll(t.Context(), &nodeapi.EnrollRequest{
		Token: parsed.Token, PublicKey: public,
		Protocol: &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor}, Platform: &nodeapi.Platform{Os: "linux", Arch: "amd64"},
	})
	if err == nil {
		node.key, node.id = private, response.GetNodeId()
	}
	return err
}

// connect runs the handshake with key and sends an empty state report,
// which opens the session to commands. It returns the stream, or the status
// that ended the handshake.
func (node *rawNode) connect(t *testing.T, key ed25519.PrivateKey) (nodeapi.NodeService_ConnectClient, error) {
	t.Helper()
	stream, _, err := node.handshake(t, key)
	if err != nil {
		return nil, err
	}
	sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: &nodeapi.StateReport{}}})
	return stream, nil
}

// handshake authenticates a session as the node instance "instance-a" and
// returns the stream and the panel's Welcome.
func (node *rawNode) handshake(t *testing.T, key ed25519.PrivateKey) (nodeapi.NodeService_ConnectClient, *nodeapi.Welcome, error) {
	t.Helper()
	stream, err := node.client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Hello{Hello: &nodeapi.Hello{
		NodeId: node.id, Protocol: &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor, Minor: node.minor},
		Platform: &nodeapi.Platform{Os: "linux", Arch: "amd64"}, BuildVersion: node.build, InstanceId: "instance-a",
	}}}); err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil {
		return nil, nil, err
	}
	challenge := frame.GetChallenge()
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Proof{
		Proof: nodeapi.SignProof(key, challenge.GetPanelId(), node.id, challenge.GetNonce()),
	}}); err != nil {
		t.Fatal(err)
	}
	frame, err = stream.Recv()
	if err != nil {
		return nil, nil, err
	}
	if frame.GetWelcome() == nil {
		t.Fatalf("expected Welcome, got %v", frame)
	}
	return stream, frame.GetWelcome(), nil
}

func sendEvent(t *testing.T, stream nodeapi.NodeService_ConnectClient, event *nodeapi.Event) {
	t.Helper()
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Event{Event: event}}); err != nil {
		t.Fatal(err)
	}
}

func requireCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("status = %v, want %v", err, want)
	}
}

func TestSessionsRequireProofAndReplaceEachOther(t *testing.T) {
	now := &clock{now: time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)}
	served := startFleet(t, now.Now)
	service, endpoint := served.service, served.endpoint
	node := &rawNode{client: dialRaw(t, endpoint, basePath, true)}

	expired, err := service.AddNode(t.Context(), "expired")
	if err != nil {
		t.Fatal(err)
	}
	now.Advance(fleet.EnrollmentLifetime + time.Second)
	requireCode(t, node.enroll(t, expired.Code), codes.Unauthenticated)

	enrollment, err := service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	_, impostor, _ := ed25519.GenerateKey(rand.Reader)
	_, err = node.connect(t, impostor)
	requireCode(t, err, codes.Unauthenticated)

	first, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, service, node.id, fleet.StateOnline)
	// The second session arrives the way a reverse proxy forwards it.
	node.client = dialRaw(t, endpoint, basePath, false)
	second, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	_, err = first.Recv()
	requireCode(t, err, codes.Aborted)

	// Enrolling again replaces the key and ends the session of the old one.
	reissued, err := service.IssueEnrollment(t.Context(), node.id)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := node.key
	if err := node.enroll(t, reissued.Code); err != nil {
		t.Fatal(err)
	}
	_, err = second.Recv()
	requireCode(t, err, codes.Unauthenticated)
	_, err = node.connect(t, oldKey)
	requireCode(t, err, codes.Unauthenticated)
	if _, err := node.connect(t, node.key); err != nil {
		t.Fatal(err)
	}
	// The node service lives only under the base path.
	_, err = dialRaw(t, endpoint, "", true).Enroll(t.Context(), &nodeapi.EnrollRequest{})
	requireCode(t, err, codes.Unimplemented)
}

func TestInstallStreamsTheCachedArchiveUnderAGrantThatEndsWithTheCommand(t *testing.T) {
	served := startFleet(t, nil)
	service := served.service
	offline := enrolledRawNode(t, served, "offline", 0, "")
	if file, err := served.configs.ConfigurationFile(t.Context(), offline.id); err != nil || file.Revision != 1 || file.Content != "{}" {
		t.Fatalf("a new node's configuration = %+v, %v; want the saved empty document", file, err)
	}
	if _, err := service.RuntimeStatus(t.Context(), offline.id); !errors.Is(err, fleet.ErrNodeOffline) {
		t.Fatalf("command to an offline node = %v, want ErrNodeOffline", err)
	}

	enrollment, err := service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	stream, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, service, node.id, fleet.StateOnline)

	// Larger than one FetchArtifact frame.
	archive := bytes.Repeat([]byte("sing-box archive "), 10_000)
	staged := filepath.Join(served.dataDir, "imports", "core-upload-test")
	if err := os.MkdirAll(filepath.Dir(staged), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	cached, err := served.cores.CacheUpload(t.Context(), staged, corecache.CoreUpload{
		SourceDescription: "test archive", ExactVersion: "1.13.19", Architecture: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}

	grants := make(chan string, 1)
	answered := make(chan error, 1)
	go func() {
		answered <- func() error {
			frame, err := stream.Recv()
			if err != nil {
				return err
			}
			command := frame.GetCommand()
			install := command.GetInstallCore()
			if install == nil || install.GetArtifact().GetId() != cached.ID || install.GetSize() != int64(len(archive)) {
				return fmt.Errorf("unexpected command %v", frame)
			}
			fetch, err := node.client.FetchArtifact(t.Context(), &nodeapi.FetchArtifactRequest{
				Grant: install.GetFetchGrant(), ArtifactId: cached.ID,
			})
			if err != nil {
				return err
			}
			var received []byte
			for {
				chunk, err := fetch.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return err
				}
				if len(received) == 0 && (chunk.GetArtifact().GetId() != cached.ID || chunk.GetSize() != int64(len(archive))) {
					return fmt.Errorf("first frame does not identify the artifact: %v", chunk.GetArtifact())
				}
				received = append(received, chunk.GetData()...)
			}
			if !bytes.Equal(received, archive) {
				return fmt.Errorf("received %d bytes, want the %d cached bytes", len(received), len(archive))
			}
			grants <- install.GetFetchGrant()
			return stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: &nodeapi.Result{
				RequestId: command.GetRequestId(),
				Outcome: &nodeapi.Result_Core{Core: &nodeapi.CoreOutcome{Installed: []*nodeapi.InstalledCore{{
					Artifact: install.GetArtifact(), BinarySha256: strings.Repeat("c", 64), BinaryPath: "/node/cores/sing-box",
					FeatureFingerprint: "{}", InstalledAt: timestamppb.Now(),
				}}}},
			}}})
		}()
	}()
	installed, err := service.InstallCore(t.Context(), node.id, cached)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-answered; err != nil {
		t.Fatal(err)
	}
	if installed.ID != cached.ID || installed.ExactVersion != "1.13.19" || installed.BinaryPath != "/node/cores/sing-box" {
		t.Fatalf("installed = %+v, want the cached artifact %s", installed, cached.ID)
	}
	fetch, err := node.client.FetchArtifact(t.Context(), &nodeapi.FetchArtifactRequest{Grant: <-grants, ArtifactId: cached.ID})
	if err == nil {
		_, err = fetch.Recv()
	}
	requireCode(t, err, codes.PermissionDenied)
}

func TestStateChangingCommandsOutliveTheirCaller(t *testing.T) {
	served := startFleet(t, nil)
	enrollment, err := served.service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	stream, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, served.service, node.id, fleet.StateOnline)

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() {
		_, err := served.service.StopRuntime(ctx, node.id)
		stopped <- err
	}()
	frame, err := stream.Recv()
	if err != nil || frame.GetCommand().GetStopRuntime() == nil {
		t.Fatalf("expected a stop command, got %v, %v", frame, err)
	}
	// The client goes away while the node is stopping the core.
	cancel()
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: &nodeapi.Result{
		RequestId: frame.GetCommand().GetRequestId(),
		Outcome:   &nodeapi.Result_Runtime{Runtime: &nodeapi.RuntimeOutcome{Status: &nodeapi.RuntimeStatus{ObservationState: "stopped"}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := <-stopped; err != nil {
		t.Fatalf("stop after the caller went away = %v, want the node's result", err)
	}
}

func TestBatchReportsEachNodesOutcome(t *testing.T) {
	served := startFleet(t, nil)
	enrollment, err := served.service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	stream, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, served.service, node.id, fleet.StateOnline)
	offline := enrolledRawNode(t, served, "offline", 0, "")
	unused, err := served.service.AddNode(t.Context(), "unused")
	if err != nil {
		t.Fatal(err)
	}
	// The online node answers its one stop command.
	answered := make(chan error, 1)
	go func() {
		frame, err := stream.Recv()
		if err == nil && frame.GetCommand().GetStopRuntime() == nil {
			err = fmt.Errorf("expected a stop command, got %v", frame)
		}
		if err == nil {
			err = stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Result{Result: &nodeapi.Result{
				RequestId: frame.GetCommand().GetRequestId(),
				Outcome:   &nodeapi.Result_Runtime{Runtime: &nodeapi.RuntimeOutcome{Status: &nodeapi.RuntimeStatus{ObservationState: "stopped"}}},
			}}})
		}
		answered <- err
	}()
	nodes, err := served.service.BatchNodes(t.Context(), fleet.BatchTarget{NodeIDs: []string{node.id, offline.id, unused.NodeID, node.id}})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := map[string]error{}
	var order []string
	served.service.RunBatch(t.Context(), nodes, func(ctx context.Context, node fleet.Node) error {
		_, err := served.service.StopRuntime(ctx, node.ID)
		return err
	}, func(result fleet.BatchResult) {
		outcomes[result.NodeID] = result.Err
		order = append(order, result.NodeID)
	})
	if err := <-answered; err != nil {
		t.Fatal(err)
	}
	// An unknown ID, here an unused code's, is reported before any node runs;
	// a repeated one runs once.
	if len(order) != 3 || order[0] != unused.NodeID || outcomes[node.id] != nil ||
		!errors.Is(outcomes[offline.id], fleet.ErrNodeOffline) || !errors.Is(outcomes[unused.NodeID], store.ErrNodeNotFound) {
		t.Fatalf("batch outcomes = %v in order %v; want the online node stopped and each other node's own failure", outcomes, order)
	}
}

// TestStatusShowsThePanelsWritesWhileStreamed enrolls and renames a node
// while a stream holds a fresh shared snapshot: the next read shows each
// change at once. A code no host used yet shows no node.
func TestStatusShowsThePanelsWritesWhileStreamed(t *testing.T) {
	served := startFleet(t, nil)
	enrollment, err := served.service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	snapshots, unsubscribe := served.service.SubscribeStatus()
	defer unsubscribe()
	if first := <-snapshots; first.Err != nil || len(first.Value.Nodes) != 0 {
		t.Fatalf("first snapshot = %+v, want no node before the code is used", first)
	}
	if _, err := served.service.FindNode(t.Context(), "edge"); !errors.Is(err, store.ErrNodeNotFound) {
		t.Fatalf("finding an unused code's name = %v, want ErrNodeNotFound", err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	status, err := served.service.Status(t.Context())
	if err != nil || len(status.Nodes) != 1 || status.Nodes[0].Node.ID != enrollment.NodeID || status.Nodes[0].Node.State != fleet.StateOffline {
		t.Fatalf("status after enrolling = %+v, %v", status.Nodes, err)
	}
	if _, err := served.service.RenameNode(t.Context(), enrollment.NodeID, "edge-renamed"); err != nil {
		t.Fatal(err)
	}
	status, err = served.service.Status(t.Context())
	if err != nil || len(status.Nodes) != 1 || status.Nodes[0].Node.Name != "edge-renamed" {
		t.Fatalf("status after the rename = %+v, %v", status.Nodes, err)
	}
}

func TestSessionWhoseFirstReportFailsEndsInsteadOfHangingCommands(t *testing.T) {
	served := startFleet(t, nil)
	service := served.service
	enrollment, err := service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	stream, _, err := node.handshake(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	// A command without a deadline of its own waits for the first report.
	failed := make(chan error, 1)
	go func() {
		_, err := service.RuntimeStatus(context.Background(), node.id)
		failed <- err
	}()
	sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: &nodeapi.StateReport{
		Host: &nodeapi.Host{PublicIp: "203.0.113.7\n"},
	}}})
	for {
		if _, err := stream.Recv(); err != nil {
			requireCode(t, err, codes.Unavailable)
			break
		}
	}
	select {
	case err := <-failed:
		if !errors.Is(err, fleet.ErrNodeOffline) {
			t.Fatalf("waiting command = %v, want node offline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the command kept waiting for a session that ended")
	}
}

func TestNodeReportsStateHistoryAndTraffic(t *testing.T) {
	served := startFleet(t, nil)
	service := served.service
	enrollment, err := service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	stream, welcome, err := node.handshake(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	if welcome.GetRuntimeHistoryAfter() != 0 || welcome.GetMetricsInterval().AsDuration() != fleet.MetricsInterval {
		t.Fatalf("first Welcome = %v", welcome)
	}
	// Commands wait for the node's state; this one gives up before it arrives.
	waiting, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := service.RuntimeStatus(waiting, node.id); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command before the first state report = %v, want it to wait", err)
	}

	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	existing := []byte(`{"log":{"level":"warn"}}`)
	sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: &nodeapi.StateReport{
		Host: &nodeapi.Host{Hostname: "edge-host", PublicIp: "203.0.113.7"},
		Runtime: &nodeapi.RuntimeStatus{ObservationState: "running", AppliedBundleId: "bundle_a", Running: &nodeapi.RunningCore{
			Pid: 42, ProcessStartToken: "start-a", ActivationBundleId: "bundle_a", StartedAt: timestamppb.New(startedAt),
		}},
		Applied:               &nodeapi.Bundle{BundleId: "bundle_a", MonitoringTier: nodeapi.MonitoringTier_MONITORING_TIER_LIMITED},
		ExistingConfiguration: &nodeapi.ConfigurationRevision{RevisionId: "rev_node", Content: existing},
	}}})
	for _, transition := range []*nodeapi.RuntimeTransition{
		{Id: 1, Status: nodeapi.ProcessState_PROCESS_STATE_UNKNOWN, Reason: "history_initialized",
			OccurredAt: timestamppb.New(startedAt.Add(-time.Hour)), UncertainSince: timestamppb.New(startedAt.Add(-time.Hour))},
		{Id: 2, Status: nodeapi.ProcessState_PROCESS_STATE_RUNNING, Reason: "start_succeeded", BundleId: "bundle_a",
			Pid: 42, ProcessStartedAt: timestamppb.New(startedAt), OccurredAt: timestamppb.New(startedAt)},
	} {
		sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_RuntimeTransition{RuntimeTransition: transition}})
	}
	sampledAt := time.Now().UTC().Add(-15 * time.Second)
	for index, totals := range [][2]uint64{{1000, 5000}, {1300, 5900}} {
		sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_MetricsSample{MetricsSample: &nodeapi.MetricsSample{
			SampledAt: timestamppb.New(sampledAt.Add(time.Duration(index) * 10 * time.Second)), CpuCount: 2,
			Traffic: &nodeapi.TrafficCounters{BundleId: "bundle_a", Pid: 42, ProcessStartToken: "start-a",
				UploadBytes: totals[0], DownloadBytes: totals[1], Connections: 3, MemoryBytes: 1 << 20},
		}}})
	}

	// The panel records events in order, so the last one settles the rest.
	var metrics telemetry.MetricsSnapshot
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		metrics, err = service.NodeMetrics(t.Context(), node.id)
		if err == nil && metrics.CurrentTrafficData.OutboundBytes == 300 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("node metrics = %+v, %v; want the delta of the two samples", metrics, err)
		}
	}
	if !metrics.Available || metrics.AppliedBundleID != "bundle_a" || metrics.CurrentTrafficData.InboundBytes != 900 ||
		metrics.LiveSample == nil || metrics.LiveSample.ActiveConnections != 3 || metrics.Host == nil || metrics.Host.CPUCount != 2 {
		t.Fatalf("node metrics = %+v", metrics)
	}
	// The fleet overview reads every node at once, its rate averaged over the
	// baseline interval, without the stored detail only the node's view reads.
	status, err := service.Status(t.Context())
	if err != nil || len(status.Nodes) != 1 || status.Nodes[0].Node.ID != node.id || status.Nodes[0].Metrics.LiveSample == nil ||
		status.Nodes[0].Metrics.LiveSample.UploadBytesPerSecond == nil || *status.Nodes[0].Metrics.LiveSample.UploadBytesPerSecond != 30 ||
		status.Nodes[0].Metrics.LatestSample != nil || status.Nodes[0].Metrics.CurrentTrafficData.Counters != nil || metrics.LatestSample == nil {
		t.Fatalf("fleet status = %+v, %v; want the node with its ten-second rate", status, err)
	}
	view, err := service.Node(t.Context(), node.id)
	if err != nil || view.ReportedAt == nil || view.PublicIP != "203.0.113.7" || view.Runtime == nil ||
		view.Runtime.ObservationState != "running" || view.Runtime.Running == nil || view.Runtime.Running.PID != 42 {
		t.Fatalf("node view = %+v, %v", view, err)
	}
	if file, err := served.configs.ConfigurationFile(t.Context(), node.id); err != nil || file.Revision != 2 || file.Content != string(existing) {
		t.Fatalf("configuration = %+v, %v; want the configuration the node runs", file, err)
	}
	history, err := served.records.RuntimeHistory(t.Context(), telemetry.RuntimeHistoryRequest{NodeID: node.id, Limit: 10})
	if err != nil || len(history.Items) != 2 || history.Items[0].Reason != "start_succeeded" || !history.HistoryStartedAt.Equal(startedAt.Add(-time.Hour)) {
		t.Fatalf("runtime history = %+v, %v", history, err)
	}

	// A viewer of live metrics raises the node to one sample a second.
	_, unsubscribe := service.SubscribeMetrics(node.id)
	for _, want := range []time.Duration{fleet.LiveMetricsInterval, fleet.MetricsInterval} {
		frame, err := stream.Recv()
		if err != nil || frame.GetTelemetryControl().GetMetricsInterval().AsDuration() != want {
			t.Fatalf("telemetry control = %v, %v; want %s", frame, err, want)
		}
		unsubscribe()
	}

	// A new session sends only the history the panel lacks.
	_ = stream.CloseSend()
	_, welcome, err = node.handshake(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	if welcome.GetRuntimeHistoryAfter() != 2 {
		t.Fatalf("runtime history after reconnecting = %d, want 2", welcome.GetRuntimeHistoryAfter())
	}
}

func TestCacheSweepKeepsTheArchivesNodesUse(t *testing.T) {
	served := startFleet(t, nil)
	cachedAt := time.Now().UTC().Add(-2 * fleet.InstallTimeout)
	cache := func(id, kind, digest string) {
		t.Helper()
		artifact := store.CachedArtifact{
			ID: id, SourceKind: kind, SHA256: digest, Size: 10, OS: "linux", Arch: "amd64", Variant: "musl",
			ExactVersion: "1.13.19", CreatedAt: cachedAt,
		}
		if kind == store.CachedArtifactOfficial {
			artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID = 1, 2, int64(len(id))
		} else {
			artifact.UserSource = "uploaded"
		}
		if _, err := served.database.RecordCachedArtifact(t.Context(), artifact); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(served.dataDir, "artifact-cache", digest+".tar.gz")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	installed, unused, uploaded := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	cache("artifact_installed", store.CachedArtifactOfficial, installed)
	cache("artifact_unused", store.CachedArtifactOfficial, unused)
	cache("artifact_uploaded", store.CachedArtifactUserVerified, uploaded)

	// A node that went offline still has what it last reported installed.
	enrollment, err := served.service.AddNode(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	node := &rawNode{client: dialRaw(t, served.endpoint, basePath, true)}
	if err := node.enroll(t, enrollment.Code); err != nil {
		t.Fatal(err)
	}
	stream, _, err := node.handshake(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: &nodeapi.StateReport{
		Installed: []*nodeapi.InstalledCore{{Artifact: &nodeapi.ArtifactIdentity{Id: "artifact_installed"}}},
	}}})
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if view, err := served.service.Node(t.Context(), node.id); err == nil && view.ReportedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the node's report was not recorded")
		}
	}
	_ = stream.CloseSend()
	waitState(t, served.service, node.id, fleet.StateOffline)

	removed, err := served.service.SweepArtifactCache(t.Context())
	if err != nil || removed != 1 {
		t.Fatalf("sweep removed %d, %v; want only the unused official archive", removed, err)
	}
	for id, kept := range map[string]bool{"artifact_installed": true, "artifact_unused": false, "artifact_uploaded": true} {
		_, err := served.database.CachedArtifact(t.Context(), id)
		if kept != (err == nil) {
			t.Fatalf("cached %s after the sweep: %v, want kept=%v", id, err, kept)
		}
	}
	if _, err := os.Stat(filepath.Join(served.dataDir, "artifact-cache", unused+".tar.gz")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the unused archive's file remains: %v", err)
	}
}
