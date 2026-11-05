// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"sync"
	"time"

	coreruntime "github.com/rehuony/sing-box-panel/internal/sing-box-node/runtime"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

const (
	// TrafficSampleInterval is how often the node reads the core's counters.
	// Traffic between the last read and a process exit is not counted.
	TrafficSampleInterval = time.Second
	// trafficCheckpointInterval bounds the traffic a node restart can lose
	// while the core keeps running.
	trafficCheckpointInterval = 10 * time.Second
	// trafficSampleLifetime is how long the latest sample stays current.
	trafficSampleLifetime = 3 * TrafficSampleInterval
)

// TrafficSample is the running core's traffic as the node accounts it.
type TrafficSample struct {
	ActivationBundleID string
	PID                int
	ProcessStartToken  string
	SampledAt          time.Time
	// UploadTotal and DownloadTotal are cumulative for the node instance
	// across core restarts.
	UploadTotal   int64
	DownloadTotal int64
	Connections   int64
	MemoryBytes   int64
}

type trafficCollector struct {
	mu         sync.Mutex
	client     *coreruntime.ClashClient
	clientFor  string
	checkpoint store.TrafficCheckpoint
	// sampled reports whether checkpoint holds a process's counters.
	sampled     bool
	loaded      bool
	persistedAt time.Time
	latest      *TrafficSample
}

// LatestTraffic returns the newest traffic sample while it is current, or nil
// when no core runs with limited monitoring or sampling fails.
func (node *Node) LatestTraffic() *TrafficSample {
	collector := &node.traffic
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.latest == nil || node.now().Sub(collector.latest.SampledAt) > trafficSampleLifetime {
		return nil
	}
	sample := *collector.latest
	return &sample
}

// RunTrafficCollection reads the running core's counters every second and
// keeps the instance's cumulative traffic until ctx ends.
func (node *Node) RunTrafficCollection(ctx context.Context) {
	ticker := time.NewTicker(TrafficSampleInterval)
	defer ticker.Stop()
	defer node.closeTrafficCollection()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			node.collectTraffic(ctx)
		}
	}
}

func (node *Node) collectTraffic(ctx context.Context) {
	collector := &node.traffic
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if !collector.loaded {
		checkpoint, ok, err := node.database.TrafficCheckpoint(ctx)
		if err != nil {
			collector.latest = nil
			return
		}
		collector.checkpoint, collector.sampled, collector.loaded = checkpoint, ok, true
		collector.persistedAt = checkpoint.SampledAt
	}
	sample, err := node.readTraffic(ctx, collector)
	if err != nil {
		collector.latest = nil
		return
	}
	next, decreased := advanceTraffic(collector.checkpoint, collector.sampled, sample)
	processChanged := !collector.sampled || next.PID != collector.checkpoint.PID ||
		next.ProcessStartToken != collector.checkpoint.ProcessStartToken
	collector.checkpoint, collector.sampled = next, true
	if processChanged || decreased || next.SampledAt.Sub(collector.persistedAt) >= trafficCheckpointInterval {
		collector.persist(ctx, node)
	}
	if decreased {
		node.record(Event{Source: SourceCore, Level: LevelWarn, Code: "traffic.counter_decreased",
			Message: "The core's traffic counters decreased within one process; the decrease was not counted"})
	}
	collector.latest = &TrafficSample{
		ActivationBundleID: next.ActivationBundleID, PID: next.PID, ProcessStartToken: next.ProcessStartToken,
		SampledAt: next.SampledAt, UploadTotal: next.UploadTotal, DownloadTotal: next.DownloadTotal,
		Connections: sample.connections, MemoryBytes: sample.memory,
	}
}

// coreTraffic is one reading of a core process's own counters.
type coreTraffic struct {
	identity    RuntimeIdentity
	sampledAt   time.Time
	upload      int64
	download    int64
	connections int64
	memory      int64
}

// readTraffic reads the counters of the verified running core when its
// applied bundle uses limited monitoring.
func (node *Node) readTraffic(ctx context.Context, collector *trafficCollector) (coreTraffic, error) {
	identity, err := node.runtime.Resolve(ctx)
	if err != nil {
		return coreTraffic{}, err
	}
	if collector.client == nil || collector.clientFor != identity.ActivationBundleID+"/"+identity.ProcessStartToken {
		if collector.client != nil {
			collector.client.CloseIdleConnections()
			collector.client = nil
		}
		material, err := node.LoadRuntimeMaterial(ctx, identity.ActivationBundleID)
		if err != nil {
			return coreTraffic{}, err
		}
		if material.Activation.MonitoringTier != store.MonitoringLimited {
			return coreTraffic{}, ErrMonitoringTierUnavailable
		}
		endpoint, err := coreruntime.ParseClashEndpoint(material.Bundle.StartupConfig)
		if err != nil {
			return coreTraffic{}, err
		}
		client, err := coreruntime.NewClashClient(endpoint)
		if err != nil {
			return coreTraffic{}, err
		}
		collector.client, collector.clientFor = client, identity.ActivationBundleID+"/"+identity.ProcessStartToken
	}
	readContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	counters, err := collector.client.Connections(readContext)
	if err != nil {
		return coreTraffic{}, err
	}
	// The counters belong to the identity only if it still runs.
	if current, err := node.runtime.Resolve(ctx); err != nil || current.ProcessStartToken != identity.ProcessStartToken {
		return coreTraffic{}, ErrNoRunningCore
	}
	return coreTraffic{
		identity: identity, sampledAt: node.now().UTC(), upload: counters.UploadTotal, download: counters.DownloadTotal,
		connections: counters.Connections, memory: counters.Memory,
	}, nil
}

// advanceTraffic adds a reading to the checkpoint. A process's first reading
// becomes its baseline and adds nothing, because traffic before it cannot be
// told from traffic already counted. A decrease within one process adds
// nothing, rebases the process and is reported.
func advanceTraffic(checkpoint store.TrafficCheckpoint, sampled bool, reading coreTraffic) (store.TrafficCheckpoint, bool) {
	next := checkpoint
	next.PID, next.ProcessStartToken = reading.identity.PID, reading.identity.ProcessStartToken
	next.ActivationBundleID, next.SampledAt = reading.identity.ActivationBundleID, reading.sampledAt
	next.ProcessUpload, next.ProcessDownload = reading.upload, reading.download
	sameProcess := sampled && checkpoint.PID == reading.identity.PID && checkpoint.ProcessStartToken == reading.identity.ProcessStartToken
	if !sameProcess {
		return next, false
	}
	if reading.upload < checkpoint.ProcessUpload || reading.download < checkpoint.ProcessDownload {
		return next, true
	}
	next.UploadTotal += reading.upload - checkpoint.ProcessUpload
	next.DownloadTotal += reading.download - checkpoint.ProcessDownload
	return next, false
}

func (collector *trafficCollector) persist(ctx context.Context, node *Node) {
	if err := node.database.SaveTrafficCheckpoint(ctx, collector.checkpoint); err == nil {
		collector.persistedAt = collector.checkpoint.SampledAt
	}
}

// closeTrafficCollection saves the latest counters and releases the client.
func (node *Node) closeTrafficCollection() {
	collector := &node.traffic
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.sampled && collector.checkpoint.SampledAt.After(collector.persistedAt) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		collector.persist(ctx, node)
	}
	if collector.client != nil {
		collector.client.CloseIdleConnections()
		collector.client = nil
	}
	collector.latest = nil
}
