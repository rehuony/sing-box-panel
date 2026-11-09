// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/service"
)

// Readings are what the node observes about itself, read on demand for its
// local console. Each part is nil while the node cannot observe it.
type Readings struct {
	Runtime *RuntimeReading
	Host    *host.Snapshot
	Traffic *TrafficReading
	// PublicIP is the node's cached public address; empty while unknown.
	PublicIP string
}

// RuntimeReading is the observed state of the node's core process.
type RuntimeReading struct {
	// State is the runtime observation state, such as running or stopped.
	State string
	// EnabledVersion is the exact version of the selected core; empty when
	// none is selected.
	EnabledVersion string
	// The running process; zero while none runs.
	PID            int
	RunningVersion string
	StartedAt      *time.Time
}

// TrafficReading is the node's cumulative core traffic as last counted.
type TrafficReading struct {
	UploadBytes   int64
	DownloadBytes int64
	// Live is the running core's counters; nil while none are read.
	Live *LiveTraffic
}

// LiveTraffic is one reading of the running core's counters.
type LiveTraffic struct {
	SampledAt   time.Time
	Connections int64
	MemoryBytes int64
}

// ReadingsSource reads the node's current readings. Nothing it reads waits
// for the network.
type ReadingsSource func(ctx context.Context) Readings

func readingsOf(node *service.Node, sampler *host.Sampler, dataDir string) ReadingsSource {
	return func(ctx context.Context) Readings {
		var readings Readings
		if status, err := node.RuntimeStatus(ctx); err == nil {
			runtime := &RuntimeReading{State: status.ObservationState}
			if status.EnabledCore != nil {
				runtime.EnabledVersion = status.EnabledCore.ExactCoreVersion
			}
			if running := status.Running; running != nil {
				started := running.StartedAt.UTC()
				runtime.PID, runtime.RunningVersion, runtime.StartedAt = running.PID, running.ExactCoreVersion, &started
			}
			readings.Runtime = runtime
		}
		readings.Host = sampler.Sample(dataDir)
		if traffic := node.LatestTraffic(); traffic != nil {
			readings.Traffic = &TrafficReading{
				UploadBytes: traffic.UploadTotal, DownloadBytes: traffic.DownloadTotal,
				Live: &LiveTraffic{SampledAt: traffic.SampledAt.UTC(), Connections: traffic.Connections, MemoryBytes: traffic.MemoryBytes},
			}
		} else if upload, download, ok := node.TrafficTotals(); ok {
			readings.Traffic = &TrafficReading{UploadBytes: upload, DownloadBytes: download}
		}
		readings.PublicIP = node.CachedPublicIP(ctx)
		return readings
	}
}
