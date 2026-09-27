// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// LiveTrafficSample is process-local evidence, not an accounted traffic delta.
// It is never added to persisted period totals.
type LiveTrafficSample struct {
	ActivationBundleID string    `json:"activation_bundle_id"`
	PID                int       `json:"pid"`
	ProcessStartToken  string    `json:"process_start_token"`
	SampledAt          time.Time `json:"sampled_at"`
	MemoryBytes        int64     `json:"memory_bytes"`
	ActiveConnections  int64     `json:"active_connections"`
	UploadTotal        int64     `json:"upload_total"`
	DownloadTotal      int64     `json:"download_total"`
	Accepted           bool      `json:"accepted"`
}

type trafficCollector struct {
	mu          sync.Mutex // One owner performs I/O; snapshot readers use the atomic pointer.
	client      *coreruntime.ClashClient
	identity    store.RuntimeObservation
	previous    *LiveTrafficSample
	persistedAt time.Time
	live        atomic.Pointer[LiveTrafficSample]
}

func sameTrafficProcess(a, b store.RuntimeObservation) bool {
	return a.PID == b.PID && a.ProcessStartToken == b.ProcessStartToken && a.ActivationBundleID == b.ActivationBundleID
}

// ClearLiveTrafficSample marks a collection gap without changing accounting.
func (a *Application) ClearLiveTrafficSample() { a.collector.live.Store(nil) }

// SampleLimitedTraffic collects every two seconds and checkpoints every ten.
// Incarnation changes and invalid evidence are persisted immediately so the
// lower persistence frequency cannot conceal a counter regression.
func (a *Application) SampleLimitedTraffic(ctx context.Context, observation store.RuntimeObservation, persist bool) (store.TrafficSampleResult, error) {
	c := &a.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	var result store.TrafficSampleResult
	success := false
	defer func() {
		if !success {
			c.live.Store(nil)
		}
	}()
	if c.client == nil || !sameTrafficProcess(c.identity, observation) {
		if c.client != nil {
			c.client.CloseIdleConnections()
			c.client = nil
		}
		material, err := a.LoadRuntimeMaterial(ctx, observation.ActivationBundleID)
		if err != nil {
			return result, err
		}
		if material.Activation.MonitoringTier != store.MonitoringLimited {
			return result, ErrMonitoringTierUnavailable
		}
		endpoint, err := coreruntime.ParseClashEndpoint(material.Bundle.StartupConfig)
		if err != nil {
			return result, err
		}
		client, err := coreruntime.NewClashClient(endpoint)
		if err != nil {
			return result, err
		}
		c.client, c.identity, c.previous, c.persistedAt = client, observation, nil, time.Time{}
	}
	counters, err := c.client.Connections(ctx)
	if err != nil {
		return result, err
	}
	current, err := a.database.RuntimeObservation(ctx)
	if err != nil || !sameTrafficProcess(current, observation) {
		return result, errors.New("runtime changed during traffic collection")
	}
	sample := &LiveTrafficSample{
		ActivationBundleID: observation.ActivationBundleID, PID: observation.PID, ProcessStartToken: observation.ProcessStartToken,
		SampledAt: a.now().UTC(), MemoryBytes: counters.Memory, ActiveConnections: counters.Connections,
		UploadTotal: counters.UploadTotal, DownloadTotal: counters.DownloadTotal, Accepted: true,
	}
	previous := c.previous
	if previous != nil && (!sample.SampledAt.After(previous.SampledAt) || sample.UploadTotal < previous.UploadTotal || sample.DownloadTotal < previous.DownloadTotal) {
		sample.Accepted = false
		// Flush the high-water evidence first; the rejected observation must not
		// establish a lower checkpoint merely because it fell between disk ticks.
		if previous.SampledAt.After(c.persistedAt) {
			if _, err := a.persistTrafficSample(ctx, previous); err != nil {
				return result, err
			}
			c.persistedAt = previous.SampledAt
		}
	}
	if persist || !sample.Accepted || c.persistedAt.IsZero() || sample.SampledAt.Sub(c.persistedAt) >= 10*time.Second {
		result, err = a.persistTrafficSample(ctx, sample)
		if err != nil {
			return result, err
		}
		sample.Accepted = result.Sample.Accepted
		if sample.Accepted {
			c.persistedAt = sample.SampledAt
		}
	}
	if sample.Accepted {
		c.previous = sample
	}
	c.live.Store(sample)
	success = true
	return result, nil
}

func (a *Application) persistTrafficSample(ctx context.Context, sample *LiveTrafficSample) (store.TrafficSampleResult, error) {
	policy, err := a.trafficAccounting(ctx)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	start, end, err := naturalTrafficPeriod(sample.SampledAt, policy.PeriodMonths)
	if err != nil {
		return store.TrafficSampleResult{}, err
	}
	return a.database.RecordTrafficSample(ctx, store.TrafficSampleInput{
		ActivationBundleID: sample.ActivationBundleID, PID: sample.PID, ProcessStartToken: sample.ProcessStartToken,
		SampledAt: sample.SampledAt, PeriodStart: start, PeriodEnd: end,
		MemoryBytes: sample.MemoryBytes, ActiveConnections: sample.ActiveConnections,
		UploadTotal: sample.UploadTotal, DownloadTotal: sample.DownloadTotal,
	})
}

// CloseTrafficCollector releases pooled idle connections at server shutdown.
func (a *Application) CloseTrafficCollector() {
	c := &a.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		c.client.CloseIdleConnections()
		c.client = nil
	}
	c.live.Store(nil)
}
