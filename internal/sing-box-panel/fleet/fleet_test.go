// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
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
	cores    *corecache.Service
	endpoint string
	dataDir  string
}

// startFleet serves a fleet and a stand-in Web server on one loopback port,
// with the node service under basePath.
func startFleet(t *testing.T, now func() time.Time) testFleet {
	t.Helper()
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
	panelSettings := settings.NewService(database, "", value, settings.Options{})
	records := telemetry.New(database, panelSettings, telemetry.Options{})
	cores := corecache.New(database, records, panelSettings, corecache.Options{DataDir: dataDir})
	service := fleet.New(database, panelSettings, records, nodeconfig.New(database, cores), cores, fleet.Options{
		Build: buildinfo.Info{Version: "test"}, Now: now,
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
	return testFleet{service: service, cores: cores, endpoint: listener.Addr().String(), dataDir: dataDir}
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
	if enrollment.Node.State != fleet.StatePending || !strings.HasPrefix(enrollment.Command, "sing-box-node enroll --code ") {
		t.Fatalf("enrollment = %+v", enrollment)
	}
	nodeDir := t.TempDir()
	build := buildinfo.Info{Version: "test"}
	enrolled, err := agent.Enroll(t.Context(), nodeDir, enrollment.Code, build)
	if err != nil {
		t.Fatal(err)
	}
	if enrolled.NodeID != enrollment.Node.ID || enrolled.Path != basePath || enrolled.Pin == "" {
		t.Fatalf("enrolled as %+v, want node %q under %s with a pin", enrolled, enrollment.Node.ID, basePath)
	}
	if _, err := agent.Enroll(t.Context(), t.TempDir(), enrollment.Code, build); err == nil {
		t.Fatal("a used enrollment code was accepted again")
	}

	ctx, cancel := context.WithCancel(t.Context())
	logs := make(chan string, 16)
	connected := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx, nodeDir, agent.Options{
			Build:     build,
			Logf:      func(format string, args ...any) { logs <- format },
			Connected: func() { connected <- struct{}{} },
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

	if err := service.DeleteNode(t.Context(), enrolled.NodeID); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case line := <-logs:
			if strings.HasPrefix(line, "session with panel") {
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("removing the node did not end its session")
		}
	}
}

// rawNode speaks the protocol directly to exercise the panel's checks.
type rawNode struct {
	client nodeapi.NodeServiceClient
	key    ed25519.PrivateKey
	id     string
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

// connect runs the handshake with key and returns the stream after Welcome,
// or the status that ended the handshake.
func (node *rawNode) connect(t *testing.T, key ed25519.PrivateKey) (nodeapi.NodeService_ConnectClient, error) {
	t.Helper()
	stream, err := node.client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Hello{Hello: &nodeapi.Hello{
		NodeId: node.id, Protocol: &nodeapi.ProtocolVersion{Major: nodeapi.ProtocolMajor}, Platform: &nodeapi.Platform{Os: "linux", Arch: "amd64"},
	}}}); err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	challenge := frame.GetChallenge()
	if err := stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Proof{
		Proof: nodeapi.SignProof(key, challenge.GetPanelId(), node.id, challenge.GetNonce()),
	}}); err != nil {
		t.Fatal(err)
	}
	frame, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	if frame.GetWelcome() == nil {
		t.Fatalf("expected Welcome, got %v", frame)
	}
	return stream, nil
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
	offline, err := service.AddNode(t.Context(), "offline")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RuntimeStatus(t.Context(), offline.Node.ID); !errors.Is(err, fleet.ErrNodeOffline) {
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
