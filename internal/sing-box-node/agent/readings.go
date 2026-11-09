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

// TrafficReading is the core's cumulative traffic for this node instance.
type TrafficReading struct {
	SampledAt     time.Time
	UploadBytes   int64
	DownloadBytes int64
	Connections   int64
	MemoryBytes   int64
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
				SampledAt: traffic.SampledAt.UTC(), UploadBytes: traffic.UploadTotal, DownloadBytes: traffic.DownloadTotal,
				Connections: traffic.Connections, MemoryBytes: traffic.MemoryBytes,
			}
		}
		readings.PublicIP = node.CachedPublicIP(ctx)
		return readings
	}
}
