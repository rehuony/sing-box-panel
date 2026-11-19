// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/httpapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
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
// SBP_BROWSER_NODES sets the number of share links in the uploaded source;
// SBP_BROWSER_FLEET seeds that many fleet nodes, none with 0, in place of the
// two default ones. Some fleet nodes run a configuration whose inbounds join
// the subscription node pool.
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
	nodeCount := 10
	if raw := os.Getenv("SBP_BROWSER_NODES"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 1 || count > 500 {
			t.Fatal("SBP_BROWSER_NODES must be in 1..500")
		}
		nodeCount = count
	}
	if fleetCount >= 0 {
		go feedReviewFleet(ctx, commands.Telemetry, seedReviewFleet(t, db, fleetCount))
	} else {
		// edge-tokyo publishes its inbounds from a configuration it ran
		// before the review server started; edge-frankfurt never connected,
		// so node-scoped pages show its offline state.
		testutil.PublishNode(t, db, "edge-tokyo", reviewInbounds, "tokyo.review.example.net", "")
		testutil.CreateNode(t, db, "edge-frankfurt")
	}
	seedReviewSubscriptions(t, commands, nodeCount)
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
		if index%3 == 0 {
			publishReviewConfiguration(t, database, id, report, &summary)
		}
		encoded, err := proto.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.RecordNodeStateReport(ctx, id, session, encoded, summary, now); err != nil {
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

// reviewInbounds is the configuration of review fleet nodes whose inbounds
// join the subscription node pool: a multi-user VLESS inbound, Trojan and
// Hysteria2, and a TUN inbound subscriptions cannot publish.
const reviewInbounds = `{"inbounds":[
 {"type":"vless","tag":"vless-reality","listen":"::","listen_port":443,"users":[{"name":"alice","uuid":"1f0e9c1c-5c0b-4d39-9a52-2f4a7d6c8e01"},{"name":"bob","uuid":"2a1b0d2d-6d1c-4e4a-8b63-3a5b8e7d9f12"}],
  "tls":{"enabled":true,"server_name":"www.example.com","reality":{"enabled":true,"handshake":{"server":"www.example.com","server_port":443},"private_key":"aGVsbG8taGVsbG8taGVsbG8taGVsbG8taGVsbG8taGk","short_id":["0123abcd"]}}},
 {"type":"trojan","tag":"trojan-ws","listen":"::","listen_port":8443,"users":[{"name":"review","password":"review-password"}],
  "tls":{"enabled":true,"server_name":"edge.review.example.net"},"transport":{"type":"ws","path":"/trojan"}},
 {"type":"hysteria2","tag":"hy2","listen":"::","listen_port":8443,"users":[{"name":"review","password":"review-password"}],
  "tls":{"enabled":true,"server_name":"edge.review.example.net"}},
 {"type":"tun","tag":"tun-in","address":["172.19.0.1/30"]}
]}`

// publishReviewConfiguration saves reviewInbounds as the node's configuration
// and makes the report name it as the applied one.
func publishReviewConfiguration(t *testing.T, database *store.Store, id string, report *nodeapi.StateReport, summary *store.NodeReport) {
	t.Helper()
	update, err := store.NewConfigurationFileUpdate(rand.Read, time.Now().UTC(), 0, reviewInbounds)
	if err != nil {
		t.Fatal(err)
	}
	file, err := database.SaveConfigurationFile(t.Context(), id, update.ExpectedRevision, update.Content, update.Revision)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := configuration.Parse([]byte(reviewInbounds))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(parsed.CanonicalJSON())
	summary.Applied.RevisionID, summary.Applied.ConfigSHA256 = file.CanonicalRevisionID, hex.EncodeToString(digest[:])
	report.Applied.RevisionId, report.Applied.ConfigurationSha256 = summary.Applied.RevisionID, summary.Applied.ConfigSHA256
}

// seedReviewSubscriptions uploads a source of linkCount share links across
// the supported protocols, with an unnamed link and a filter that drops
// expired ones, and a channel with two strategy groups over the pool.
func seedReviewSubscriptions(t *testing.T, services app.Services, linkCount int) {
	t.Helper()
	ctx := t.Context()
	regions := []string{"🇭🇰 Hong Kong", "🇯🇵 Tokyo", "🇸🇬 Singapore", "🇺🇸 Los Angeles", "🇩🇪 Frankfurt"}
	reality := strings.Repeat("A", 43)
	userInfo := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:review-password"))
	links := []string{}
	for index := range linkCount {
		name := url.PathEscape(fmt.Sprintf("%s %02d", regions[index%len(regions)], index/len(regions)+1))
		host := fmt.Sprintf("node-%03d.review.example.net", index+1)
		switch index % 10 {
		case 0:
			links = append(links, fmt.Sprintf("ss://%s@%s:8388#%s", userInfo, host, name))
		case 1:
			payload, _ := json.Marshal(map[string]string{"v": "2", "ps": fmt.Sprintf("%s %02d", regions[index%len(regions)], index/len(regions)+1),
				"add": host, "port": "443", "id": "3b2c1e3e-7e2d-4f5b-9c74-4b6c9f8e0a23", "aid": "0", "scy": "auto", "net": "ws", "path": "/vmess", "host": host, "tls": "tls"})
			links = append(links, "vmess://"+base64.StdEncoding.EncodeToString(payload))
		case 2:
			links = append(links, fmt.Sprintf("vless://4c3d2f4f-8f3e-4a6c-8d85-5c7d0a9f1b34@%s:443?security=reality&sni=www.example.com&pbk=%s&sid=0123abcd&fp=chrome&flow=xtls-rprx-vision#%s", host, reality, name))
		case 3:
			links = append(links, fmt.Sprintf("trojan://review-password@%s:443?sni=%s&type=grpc&serviceName=trojan#%s", host, host, name))
		case 4:
			links = append(links, fmt.Sprintf("hy2://review-password@%s:8443?sni=%s#%s", host, host, name))
		case 5:
			links = append(links, fmt.Sprintf("tuic://5d4e3a5a-9a4f-4b7d-9e96-6d8e1b0a2c45:review-password@%s:443?congestion_control=bbr&sni=%s#%s", host, host, name))
		case 6:
			links = append(links, fmt.Sprintf("socks5://review:review-password@%s:1080#%s", host, name))
		case 7:
			links = append(links, fmt.Sprintf("https://review:review-password@%s:443#%s", host, name))
		case 8:
			links = append(links, fmt.Sprintf("anytls://review-password@%s:443?sni=%s#%s", host, host, name))
		default:
			// Unnamed: the pool names it by protocol, server and port.
			links = append(links, fmt.Sprintf("hysteria://%s:443?auth=review-token&sni=%s", host, host))
		}
	}
	links = append(links, "trojan://review-password@expired.review.example.net:443#Traffic%20reset%20%C2%B7%20expired")
	content := strings.Join(links, "\n")
	source, err := services.Subscriptions.CreateSubscriptionSource(ctx, subscription.CreateSubscriptionSourceRequest{
		Name: "Review provider", Kind: store.SubscriptionSourceFile,
		Filter: &subscription.SourceFilter{Include: []string{}, Exclude: []string{"expired"}}, Content: &content,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Fleet nodes were published directly in the store.
	if err := services.Subscriptions.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := services.Subscriptions.SubscriptionNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	proxy := []map[string]string{}
	auto := []map[string]string{}
	for _, node := range pool.Items {
		if node.Origin.SourceID == source.ID && len(auto) < 4 {
			auto = append(auto, map[string]string{"kind": "node", "id": node.ID})
		}
		if node.Origin.Kind == "inbound" || len(proxy) < 6 {
			proxy = append(proxy, map[string]string{"kind": "node", "id": node.ID})
		}
	}
	proxy = append([]map[string]string{{"kind": "group", "id": "auto"}}, append(proxy, map[string]string{"kind": "builtin", "id": "direct"})...)
	config, err := json.Marshal(map[string]any{
		"incompatible_nodes": "skip",
		"groups": []any{
			map[string]any{"id": "proxy", "name": "Proxy", "enabled": true, "type": "select", "members": proxy, "rules": []any{
				map[string]any{"id": "google", "enabled": true, "kind": "domain_suffix", "value": "google.com", "exit": map[string]any{"kind": "group-default"}},
				map[string]any{"id": "lan", "enabled": true, "kind": "ip_cidr", "value": "192.168.0.0/16", "exit": map[string]any{"kind": "direct"}},
			}},
			map[string]any{"id": "auto", "name": "Auto", "enabled": true, "type": "url-test", "members": auto, "rules": []any{}},
		},
		"default_exit": map[string]any{"kind": "group", "id": "proxy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.Subscriptions.CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{
		Name: "Review channel", Format: store.SubscriptionFormatMihomo, Config: config, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
}
