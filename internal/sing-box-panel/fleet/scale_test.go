// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// Scale run parameters: each node sends scaleSamples traffic samples, one
// every scaleCadence of wall time and scaleSpacing apart in sample time, the
// node baseline, and a state report every tenth sample. The panel's clock runs
// scaleSpacing/scaleCadence times faster than wall time, as fast as the
// nodes' sample time, so the panel takes every sample as live and stores it.
const (
	scaleSamples = 30
	scaleCadence = time.Second
	scaleSpacing = 10 * time.Second
)

// TestFleetAtScale runs fleets of simulated nodes over the node listener and
// records what the fleet views cost. It is opt-in: SBP_FLEET_SCALE lists the
// node counts, such as 200,500.
func TestFleetAtScale(t *testing.T) {
	counts := os.Getenv("SBP_FLEET_SCALE")
	if counts == "" {
		t.Skip("set SBP_FLEET_SCALE to node counts, such as 200,500, to run the fleet scale test")
	}
	for _, field := range strings.Split(counts, ",") {
		count, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || count < 1 {
			t.Fatalf("SBP_FLEET_SCALE has %q, want node counts", field)
		}
		t.Run(fmt.Sprintf("%d nodes", count), func(t *testing.T) { runFleetAtScale(t, count) })
	}
}

// scaleNode is a simulated node with a running core.
type scaleNode struct {
	*rawNode
	stream *nodeapi.ConnectClient
	index  int
}

func (node *scaleNode) bundle() string { return fmt.Sprintf("bundle-%d", node.index) }

func (node *scaleNode) report() *nodeapi.StateReport {
	return &nodeapi.StateReport{
		Host:    &nodeapi.Host{Hostname: fmt.Sprintf("host-%d", node.index), Platform: &nodeapi.Platform{Os: "linux", Arch: "amd64"}, PublicIp: "203.0.113.7"},
		Applied: &nodeapi.Bundle{BundleId: node.bundle(), RevisionId: "revision", ConfigurationSha256: strings.Repeat("a", 64), ArtifactId: "artifact", ExactCoreVersion: "1.13.19"},
		Runtime: &nodeapi.RuntimeStatus{ObservationState: "running", AppliedBundleId: node.bundle(), DesiredRunning: true, Running: &nodeapi.RunningCore{
			Pid: int64(1000 + node.index), ProcessStartToken: "token", ExactCoreVersion: "1.13.19", CoreArtifactId: "artifact",
			ActivationBundleId: node.bundle(), StartedAt: timestamppb.Now(),
		}},
	}
}

// counters are the node's cumulative traffic at its sample number.
func (node *scaleNode) counters(sample int) (upload, download uint64) {
	upload = uint64(sample*(sample+1)/2) * uint64(1000+node.index)
	return upload, 2 * upload
}

// sample is the node's sample number, the first taken at base.
func (node *scaleNode) sample(base time.Time, number int) *nodeapi.MetricsSample {
	upload, download := node.counters(number)
	cpu, load, total, used := float64(number%100), 0.5, uint64(8<<30), uint64(3<<30)
	return &nodeapi.MetricsSample{
		SampledAt: timestamppb.New(base.Add(time.Duration(number-1) * scaleSpacing)), CpuPercent: &cpu, CpuCount: 4, LoadOne: &load,
		MemoryTotal: &total, MemoryUsed: &used, DiskTotal: &total, DiskUsed: &used,
		Traffic: &nodeapi.TrafficCounters{
			BundleId: node.bundle(), UploadBytes: upload, DownloadBytes: download, Connections: uint32(number),
			Pid: int64(1000 + node.index), ProcessStartToken: "token", MemoryBytes: 64 << 20,
		},
	}
}

func heapInUse() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func runFleetAtScale(t *testing.T, count int) {
	ctx := t.Context()
	heapBefore := heapInUse()
	// The panel's clock starts mid-month, so a run never crosses a traffic
	// period, and keeps pace with the nodes' sample time: the panel replaces
	// a sample time that strays from its own clock.
	wallStart := time.Now()
	panelStart := time.Date(wallStart.UTC().Year(), wallStart.UTC().Month(), 15, 0, 0, 0, 0, time.UTC)
	speedup := scaleSpacing / scaleCadence
	served := startFleetWith(t, fleetSetup{now: func() time.Time { return panelStart.Add(time.Since(wallStart) * speedup) }})
	nodes := make([]*scaleNode, count)
	started := time.Now()
	for index := range nodes {
		raw := enrolledRawNode(t, served, fmt.Sprintf("node-%04d", index), 1, "test")
		node := &scaleNode{rawNode: raw, index: index}
		stream, _, err := raw.handshake(t, raw.key)
		if err != nil {
			t.Fatal(err)
		}
		node.stream = stream
		// A node reads its session the whole time, which answers the
		// panel's keepalive pings.
		go func() {
			for {
				if _, err := stream.Recv(); err != nil {
					return
				}
			}
		}()
		sendEvent(t, stream, &nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: node.report()}})
		nodes[index] = node
	}
	for _, node := range nodes {
		waitState(t, served.service, node.id, fleet.StateOnline)
	}
	t.Logf("enrolled and connected %d nodes in %s", count, time.Since(started).Round(time.Millisecond))

	// Every snapshot read after the first is one query for the nodes and
	// memory for the rest. The old design read three more rows per node; a
	// snapshot that read even one per node would cost at least what reading
	// one such row for every node costs, measured here on the same store.
	reads := make([]time.Duration, 0, 4)
	for range 4 {
		time.Sleep(fleet.StatusStreamInterval + 100*time.Millisecond)
		start := time.Now()
		snapshot, err := served.service.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reads = append(reads, time.Since(start))
		if len(snapshot.Nodes) != count {
			t.Fatalf("snapshot has %d nodes, want %d", len(snapshot.Nodes), count)
		}
	}
	warm := slices.Min(reads[1:])
	perNodeStart := time.Now()
	for _, node := range nodes {
		if _, err := served.database.Node(ctx, node.id); err != nil {
			t.Fatal(err)
		}
	}
	perNodeReads := time.Since(perNodeStart)
	if warm >= perNodeReads {
		t.Errorf("a warm snapshot took %s, not less than one row read per node (%s): reads per node are back", warm, perNodeReads)
	}
	t.Logf("snapshot build: first %s, warm %s (%s a node); one row read per node: %s", reads[0].Round(time.Microsecond),
		warm.Round(time.Microsecond), (warm / time.Duration(count)).Round(10*time.Nanosecond), perNodeReads.Round(time.Microsecond))

	// One stream client, the way /fleet/stream follows the shared snapshot.
	updates, unsubscribe := served.service.SubscribeStatus()
	defer unsubscribe()
	changes := fleet.NewStatusChanges(fleet.StatusFilter{})
	type frameSize struct {
		at    time.Time
		bytes int
		items int
	}
	var frames []frameSize
	var framesMu sync.Mutex
	streaming, stopStreaming := context.WithCancel(ctx)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for {
			select {
			case <-streaming.Done():
				return
			case result := <-updates:
				if result.Err != nil {
					t.Errorf("stream read: %v", result.Err)
					return
				}
				if frame, changed := changes.Next(result.Value); changed {
					encoded, _ := json.Marshal(frame)
					framesMu.Lock()
					frames = append(frames, frameSize{at: time.Now(), bytes: len(encoded), items: len(frame.Items)})
					framesMu.Unlock()
				}
			}
		}
	}()

	// Every node changes on every sample. A node sends sample n no earlier
	// than n-1 cadences after sendStart, when the panel's clock has reached
	// its sample time.
	sendStart := time.Now()
	base := panelStart.Add(sendStart.Sub(wallStart) * speedup)
	heapLoaded := uint64(0)
	var senders sync.WaitGroup
	for _, node := range nodes {
		senders.Go(func() {
			ticker := time.NewTicker(scaleCadence)
			defer ticker.Stop()
			for number := 1; number <= scaleSamples; number++ {
				if err := node.stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Event{Event: &nodeapi.Event{
					Body: &nodeapi.Event_MetricsSample{MetricsSample: node.sample(base, number)},
				}}}); err != nil {
					t.Errorf("node %d sample %d: %v", node.index, number, err)
					return
				}
				if number%10 == 0 {
					if err := node.stream.Send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Event{Event: &nodeapi.Event{
						Body: &nodeapi.Event_StateReport{StateReport: node.report()},
					}}}); err != nil {
						t.Errorf("node %d report: %v", node.index, err)
						return
					}
				}
				<-ticker.C
			}
		})
	}
	time.Sleep(scaleSamples * scaleCadence / 2)
	heapLoaded = heapInUse()
	senders.Wait()
	sendTime := time.Since(sendStart)

	// No accepted sample is lost: each node's period holds the traffic of
	// every sample after its first.
	deadline := time.Now().Add(time.Minute)
	for _, node := range nodes {
		record, err := served.database.Node(ctx, node.id)
		if err != nil {
			t.Fatal(err)
		}
		at := base.Add(scaleSamples * scaleSpacing)
		start, end, err := telemetry.TrafficPeriod(at, record.Settings.Traffic)
		if err != nil {
			t.Fatal(err)
		}
		firstUpload, firstDownload := node.counters(1)
		lastUpload, lastDownload := node.counters(scaleSamples)
		want := int64(lastUpload-firstUpload) + int64(lastDownload-firstDownload)
		for {
			period, err := served.database.AggregateTrafficPeriod(ctx, node.id, start, end, at)
			if err != nil {
				t.Fatal(err)
			}
			if got := period.InboundBytes + period.OutboundBytes; got == want {
				break
			} else if time.Now().After(deadline) {
				t.Fatalf("node %d recorded %d bytes, want %d: accepted samples were lost", node.index, got, want)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	time.Sleep(2 * fleet.StatusStreamInterval)
	stopStreaming()
	<-streamDone

	framesMu.Lock()
	defer framesMu.Unlock()
	if len(frames) < 2 {
		t.Fatalf("%d stream frames, want the first and changes", len(frames))
	}
	first := frames[0]
	var steady []frameSize
	for _, frame := range frames[1:] {
		if frame.at.Before(sendStart.Add(sendTime)) && frame.at.After(sendStart.Add(2*time.Second)) {
			steady = append(steady, frame)
		}
	}
	bytes, items := 0, 0
	for _, frame := range steady {
		bytes += frame.bytes
		items += frame.items
	}
	if len(steady) == 0 {
		t.Fatal("no stream frames while the nodes sent samples")
	}
	window := steady[len(steady)-1].at.Sub(steady[0].at)
	t.Logf("first stream frame: %d bytes for %d nodes (%d bytes a node)", first.bytes, first.items, first.bytes/max(first.items, 1))
	t.Logf("steady stream: %d frames in %s, %.2f frames/s, %d bytes and %d items a frame on average",
		len(steady), window.Round(time.Millisecond), float64(len(steady)-1)/window.Seconds(), bytes/len(steady), items/len(steady))
	t.Logf("samples: %d sent in %s (%.0f a second), every one accepted", count*scaleSamples, sendTime.Round(time.Millisecond),
		float64(count*scaleSamples)/sendTime.Seconds())
	t.Logf("heap in use: %.1f MiB before, %.1f MiB with %d connected nodes sending (panel and simulated nodes together)",
		float64(heapBefore)/(1<<20), float64(heapLoaded)/(1<<20), count)
}
