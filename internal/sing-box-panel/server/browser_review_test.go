// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/httpapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

// TestBrowserReview serves the real HTTP boundary and built web assets against
// an isolated database for manual browser verification on development hosts.
// It deliberately has no runtime executor: it cannot validate process lifecycle.
// Without a seeded fleet, nodes can enroll and connect on the same port, as
// with the real server; a seeded fleet's sessions are recorded directly, and
// the node service would clear them when it starts.
// Run with SBP_BROWSER_REVIEW=1 go test ./internal/sing-box-panel/server -run '^TestBrowserReview$' -v -timeout 30m.
// SBP_BROWSER_NODES sets the number of subscription nodes; SBP_BROWSER_FLEET
// seeds that many fleet nodes, none with 0, in place of the two default ones.
func TestBrowserReview(t *testing.T) {
	if os.Getenv("SBP_BROWSER_REVIEW") != "1" {
		t.Skip("opt-in browser review")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	reviewRoot := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(reviewRoot, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	configuration := settings.Defaults()
	configuration.Auth.Email = testutil.AdminEmail
	configuration.Auth.PasswordHash = testutil.PasswordHash
	configuration.Server.Host = "127.0.0.1"
	configuration.Server.Port = 3337
	configuration.Server.ExternalOrigin = "http://127.0.0.1:3337"
	configuration.DataDir = reviewRoot
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(reviewRoot, "setting.json")
	if err := settings.WriteAtomic(configPath, raw); err != nil {
		t.Fatal(err)
	}
	configuration, err = settings.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	fleetCount := -1
	if raw := os.Getenv("SBP_BROWSER_FLEET"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 0 || count > 1000 {
			t.Fatal("SBP_BROWSER_FLEET must be in 0..1000")
		}
		fleetCount = count
	}
	build := buildinfo.Info{Version: "browser-review"}
	if fleetCount >= 0 {
		// A release version, so nodes that run another one offer an update.
		build.Version = reviewRelease
	}
	commands := app.New(db, configuration, app.Options{Build: build})
	nodeCount := 2
	if raw := os.Getenv("SBP_BROWSER_NODES"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 1 || count > 500 {
			t.Fatal("SBP_BROWSER_NODES must be in 1..500")
		}
		nodeCount = count
	}
	for index := range nodeCount {
		value := fmt.Sprintf(`{"type":"socks","tag":"Review node %03d","server":"node-%03d.example.com","server_port":1080}`, index+1, index+1)
		if _, err := commands.Subscriptions.CreateSubscriptionNode(ctx, []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if fleetCount >= 0 {
		go feedReviewFleet(ctx, commands.Telemetry, seedReviewFleet(t, db, fleetCount))
	} else {
		// Fleet nodes that never connected, so node-scoped pages show their
		// offline states.
		for _, name := range []string{"edge-tokyo", "edge-frankfurt"} {
			testutil.CreateNode(t, db, name)
		}
	}
	handler := httpapi.NewHandler(httpapi.HandlerOptions{
		Settings: configuration, Build: build, Services: commands,
		Assets: os.DirFS("../../../web/apps/panel/dist"),
	})
	listener, err := net.Listen("tcp", "127.0.0.1:3337")
	if err != nil {
		t.Fatal(err)
	}
	web := listener
	if fleetCount <= 0 {
		var nodes fleet.NodeListeners
		web, nodes = fleet.SplitListener(listener)
		go func() { _ = commands.Fleet.Serve(ctx, nodes) }()
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() { <-ctx.Done(); _ = server.Close() }()
	t.Log("Browser review: http://127.0.0.1:3337/panel (isolated database; no runtime executor)")
	if err := server.Serve(web); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

// reviewRelease is the panel release a seeded review fleet runs against.
const reviewRelease = "v0.9.1"

// reviewNode is a seeded node the review server shows online, with the
// totals its simulated core has counted.
type reviewNode struct {
	id, instance     string
	index            int
	upload, download int64
}

// seedReviewFleet enrolls count fleet nodes in mixed states and returns the
// online ones: every other one has a recorded session and a state report,
// which the review server shows online, and the others went offline at
// increasing times. Every fourth has a long name, every node carries one to
// three tags, every eighth runs an older release, and online nodes cycle
// through no quota, a reached quota, a nearly reached one and a comfortable
// one.
func seedReviewFleet(t *testing.T, database *store.Store, count int) []*reviewNode {
	t.Helper()
	ctx, now := t.Context(), time.Now().UTC()
	regions := []string{"hk", "jp", "sg", "us", "de", "uk"}
	var online []*reviewNode
	for index := range count {
		region := regions[index%len(regions)]
		name := fmt.Sprintf("%s-edge-%03d", region, index+1)
		if index%4 == 3 {
			name = fmt.Sprintf("%s-equinix-premium-relay-%03d-with-a-rather-long-name", region, index+1)
		}
		arch, version := "amd64", reviewRelease
		if index%2 == 1 {
			arch = "arm64"
		}
		if index%8 == 0 {
			version = "v0.9.0"
		}
		id, key := testutil.EnrollNode(t, database, name, store.NodeEnrollment{
			Hostname: name + ".review.example.net", OS: "linux", Arch: arch,
			BuildVersion: version, EnrolledAt: now.Add(-time.Duration(index) * time.Hour),
		})
		tags := []string{region}
		if index%4 == 3 {
			tags = append(tags, "premium")
		}
		if index%3 == 0 {
			tags = append(tags, "cn2")
		}
		slices.Sort(tags)
		var quota *int64
		switch index % 10 {
		case 0:
			quota = new(int64(1))
		case 2:
			quota = new(int64(2))
		case 4:
			quota = new(int64(500))
		}
		if _, err := database.SaveNodeSettings(ctx, id, 1, store.NodeSettings{
			Traffic: store.NodeTrafficSettings{PeriodMonths: 1, ResetDay: 1, QuotaGiB: quota}, Tags: tags,
		}); err != nil {
			t.Fatal(err)
		}
		seen := now.Add(-time.Duration(index*7) * time.Minute)
		if index%2 == 1 {
			if err := database.TouchNode(ctx, id, key, seen); err != nil {
				t.Fatal(err)
			}
			continue
		}
		session := fmt.Sprintf("session_review_%03d", index+1)
		if err := database.RecordNodeSession(ctx, id, key, store.NodeSession{
			ID: session, InstanceID: "instance_" + session, OS: "linux", Arch: arch, BuildVersion: version, SeenAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		report, summary := reviewReport(index, now)
		encoded, err := proto.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.RecordNodeStateReport(ctx, id, session, encoded, summary, now); err != nil {
			t.Fatal(err)
		}
		online = append(online, &reviewNode{id: id, instance: "instance_" + session, index: index})
	}
	return online
}

// reviewReport is what an online review node reports: a running core, or
// every tenth a stopped one, under one of two core versions.
func reviewReport(index int, now time.Time) (*nodeapi.StateReport, store.NodeReport) {
	core, bundle := "1.13.19", fmt.Sprintf("bundle_review_%03d", index+1)
	if index%3 == 1 {
		core = "1.12.8"
	}
	runtime := &nodeapi.RuntimeStatus{ObservationState: "running", AppliedBundleId: bundle, DesiredRunning: true, Running: &nodeapi.RunningCore{
		Pid: int64(1000 + index), ProcessStartToken: "token", ExactCoreVersion: core, CoreArtifactId: "artifact",
		ActivationBundleId: bundle, StartedAt: timestamppb.New(now.Add(-time.Duration(index*97) * time.Minute)),
	}}
	if index%10 == 6 {
		runtime = &nodeapi.RuntimeStatus{ObservationState: "stopped", AppliedBundleId: bundle}
	}
	ip := fmt.Sprintf("203.0.113.%d", index%250+1)
	report := &nodeapi.StateReport{
		Host:    &nodeapi.Host{PublicIp: ip, Platform: &nodeapi.Platform{Os: "linux", Arch: "amd64"}},
		Applied: &nodeapi.Bundle{BundleId: bundle, RevisionId: "revision", ConfigurationSha256: strings.Repeat("a", 64), ExactCoreVersion: core},
		Runtime: runtime,
	}
	return report, store.NodeReport{PublicIP: ip, Applied: store.AppliedBundle{ID: bundle, RevisionID: "revision", ExactCoreVersion: core}}
}

// feedReviewFleet sends every online review node a sample every five
// seconds: host load that reaches the warning and danger thresholds on some
// nodes, and, while the core runs, traffic at the node's own rate. The first
// two samples count each quota's share, so quotas read reached, nearly
// reached or comfortable.
func feedReviewFleet(ctx context.Context, records *telemetry.Service, nodes []*reviewNode) {
	sample := func(node *reviewNode, at time.Time) {
		cpu, load, memory, disk := float64(node.index*37%100), float64(node.index%9)/2, uint64(8<<30), uint64(64<<30)
		memoryUsed, diskUsed := memory*uint64(node.index*53%100)/100, disk*uint64(node.index*71%100)/100
		value := telemetry.NodeSample{SampledAt: at, Host: &telemetry.HostMetrics{
			SampledAt: at, CPUPercent: &cpu, CPUCount: 4, LoadOne: &load,
			MemoryTotal: &memory, MemoryUsed: &memoryUsed, DiskTotal: &disk, DiskUsed: &diskUsed,
		}}
		if node.index%10 != 6 {
			value.Traffic = &telemetry.NodeTraffic{
				ActivationBundleID: fmt.Sprintf("bundle_review_%03d", node.index+1), PID: 1000 + node.index, ProcessStartToken: "token",
				UploadTotal: node.upload, DownloadTotal: node.download, Connections: int64(node.index * 3 % 200), MemoryBytes: 64 << 20,
			}
		}
		_ = records.RecordNodeSample(ctx, node.id, node.instance, value)
	}
	start := time.Now().Add(-3 * time.Minute)
	for _, node := range nodes {
		sample(node, start)
		switch node.index % 10 {
		case 0:
			node.download = 1288 << 20
		case 2:
			node.download = 1894 << 20
		default:
			node.download = int64(node.index+1) << 24
		}
		node.upload = node.download / 4
		sample(node, time.Now().Add(-20*time.Second))
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			for _, node := range nodes {
				rate := int64(node.index%7+1) << 17
				node.download += rate * 5
				node.upload += rate * 5 / 3
				sample(node, at)
			}
		}
	}
}
