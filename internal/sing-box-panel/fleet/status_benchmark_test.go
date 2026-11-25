// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// reportingFleet is a fleet whose nodes each reported their state and two
// traffic samples, without a listener.
type reportingFleet struct {
	service *fleet.Service
	records *telemetry.Service
	nodes   []reportingNode
	at      time.Time
}

type reportingNode struct {
	id, instanceID string
}

func (seeded reportingFleet) sample(index int, at time.Time, total int64) telemetry.NodeSample {
	return telemetry.NodeSample{SampledAt: at, Traffic: &telemetry.NodeTraffic{
		ActivationBundleID: fmt.Sprintf("bundle-%d", index), PID: 100 + index, ProcessStartToken: "token",
		UploadTotal: total, DownloadTotal: 2 * total, Connections: 3, MemoryBytes: 1 << 20,
	}}
}

func seedReportingFleet(b *testing.B, count int) reportingFleet {
	b.Helper()
	ctx := context.Background()
	dataDir := b.TempDir()
	database, err := store.Open(ctx, filepath.Join(dataDir, "panel.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = database.Close() })
	at := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return at }
	value := settings.Defaults()
	value.DataDir = dataDir
	value.Nodes.Endpoint = "127.0.0.1:3000"
	panelSettings := settings.NewService(database, "", value)
	records := telemetry.New(database, panelSettings, telemetry.Options{Now: now})
	cores := corecache.New(database, records, panelSettings, corecache.Options{DataDir: dataDir})
	service := fleet.New(database, panelSettings, records, nodeconfig.New(database, cores, nil), cores, fleet.Options{
		Build: buildinfo.Info{Version: "test"}, Now: now,
	})
	seeded := reportingFleet{service: service, records: records, at: at}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	for index := range count {
		enrollment, err := service.AddNode(ctx, fmt.Sprintf("node-%04d", index))
		if err != nil {
			b.Fatal(err)
		}
		code, err := nodeapi.ParseEnrollmentCode(enrollment.Code)
		if err != nil {
			b.Fatal(err)
		}
		id := enrollment.NodeID
		if _, _, err := database.EnrollNode(ctx, nodeapi.TokenHash(code.Token), store.NodeEnrollment{
			PublicKey: publicKey, Hostname: "host", OS: "linux", Arch: "amd64", BuildVersion: "test", EnrolledAt: at,
		}); err != nil {
			b.Fatal(err)
		}
		node := reportingNode{id: id, instanceID: fmt.Sprintf("instance-%d", index)}
		session := fmt.Sprintf("session-%d", index)
		if err := database.RecordNodeSession(ctx, id, publicKey, store.NodeSession{
			ID: session, InstanceID: node.instanceID, OS: "linux", Arch: "amd64", BuildVersion: "test", SeenAt: at,
		}); err != nil {
			b.Fatal(err)
		}
		bundle := fmt.Sprintf("bundle-%d", index)
		report, err := proto.Marshal(&nodeapi.StateReport{
			Host:    &nodeapi.Host{Hostname: "host", PublicIp: "203.0.113.7"},
			Applied: &nodeapi.Bundle{BundleId: bundle, MonitoringTier: nodeapi.MonitoringTier_MONITORING_TIER_LIMITED, ExactCoreVersion: "1.14.3"},
			Runtime: &nodeapi.RuntimeStatus{
				ObservationState: "running", AppliedBundleId: bundle,
				Running: &nodeapi.RunningCore{Pid: int64(100 + index), ProcessStartToken: "token", ActivationBundleId: bundle, ExactCoreVersion: "1.14.3"},
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := database.RecordNodeStateReport(ctx, id, session, report, store.NodeReport{
			PublicIP: "203.0.113.7", Applied: store.AppliedBundle{ID: bundle, ExactCoreVersion: "1.14.3"},
		}, at); err != nil {
			b.Fatal(err)
		}
		// Two stored samples ten seconds apart prove a delta and a live rate.
		for step, total := range []int64{1000, 5000} {
			sampledAt := at.Add(time.Duration(step-1) * 10 * time.Second)
			if err := records.RecordNodeSample(ctx, id, node.instanceID, seeded.sample(index, sampledAt, total)); err != nil {
				b.Fatal(err)
			}
		}
		seeded.nodes = append(seeded.nodes, node)
	}
	return seeded
}

// BenchmarkFleetStatus measures one read of every node with its metrics, the
// read behind the fleet views.
func BenchmarkFleetStatus(b *testing.B) {
	for _, count := range []int{50, 200, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			seeded := seedReportingFleet(b, count)
			ctx := context.Background()
			// One read fills what the stored samples did not, so the loop
			// measures the steady state.
			if _, err := seeded.service.Status(ctx); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				status, err := seeded.service.Status(ctx)
				if err != nil || len(status.Nodes) != count {
					b.Fatalf("status: %d nodes, %v", len(status.Nodes), err)
				}
			}
		})
	}
}

// BenchmarkRecordNodeSample measures storing one traffic sample, the write
// every reporting node causes at the persistence interval.
func BenchmarkRecordNodeSample(b *testing.B) {
	seeded := seedReportingFleet(b, 200)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for step := range b.N {
		index := step % len(seeded.nodes)
		node := seeded.nodes[index]
		// Each round is ten seconds after the last, so every sample is stored.
		at := seeded.at.Add(time.Duration(step/len(seeded.nodes)+1) * 10 * time.Second)
		if err := seeded.records.RecordNodeSample(ctx, node.id, node.instanceID, seeded.sample(index, at, int64(5000+step))); err != nil {
			b.Fatal(err)
		}
	}
}
