// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/service"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

const (
	// historyPollInterval is how often the node looks for new runtime
	// transitions; a new transition also refreshes the state report.
	historyPollInterval = time.Second
	// stateRefreshInterval catches state that changes without a transition,
	// such as the public IP.
	stateRefreshInterval = 30 * time.Second
	historyBatchSize     = 100
	minimumMetricsPeriod = time.Second
	maximumMetricsPeriod = time.Hour
)

// reporter sends the node's state, runtime history and metrics over one
// session.
type reporter struct {
	node     *service.Node
	database *store.Store
	sampler  *host.Sampler
	dataDir  string
	send     func(*nodeapi.ConnectRequest) error
	interval chan time.Duration

	mu sync.Mutex
	// lastReport is the encoded state report the panel last received.
	lastReport []byte
	// historyAfter is the newest runtime transition the panel has.
	historyAfter int64
}

func newReporter(node *service.Node, database *store.Store, sampler *host.Sampler, dataDir string,
	send func(*nodeapi.ConnectRequest) error, historyAfter int64,
) *reporter {
	return &reporter{
		node: node, database: database, sampler: sampler, dataDir: dataDir, send: send,
		interval: make(chan time.Duration, 1), historyAfter: historyAfter,
	}
}

// start sends the first state report, which carries the applied
// configuration, and the runtime history the panel lacks.
func (reporter *reporter) start(ctx context.Context) error {
	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	report, err := reporter.stateReport(ctx)
	if err != nil {
		return err
	}
	encoded, err := encodeReport(report)
	if err != nil {
		return err
	}
	if report.Applied != nil {
		if report.ExistingConfiguration, err = reporter.appliedConfiguration(ctx, report.Applied.GetRevisionId()); err != nil {
			return err
		}
	}
	if err := reporter.sendEvent(&nodeapi.Event{Body: &nodeapi.Event_StateReport{StateReport: report}}); err != nil {
		return err
	}
	reporter.lastReport = encoded
	_, err = reporter.sendHistoryLocked(ctx)
	return err
}

// run keeps the panel informed until ctx ends or a send fails.
func (reporter *reporter) run(ctx context.Context, metricsInterval time.Duration) error {
	metrics := time.NewTicker(boundedInterval(metricsInterval))
	defer metrics.Stop()
	history := time.NewTicker(historyPollInterval)
	defer history.Stop()
	refresh := time.NewTicker(stateRefreshInterval)
	defer refresh.Stop()
	for {
		var err error
		select {
		case <-ctx.Done():
			return nil
		case interval := <-reporter.interval:
			metrics.Reset(boundedInterval(interval))
			err = reporter.sendMetrics(ctx)
		case <-metrics.C:
			err = reporter.sendMetrics(ctx)
		case <-history.C:
			reporter.mu.Lock()
			var sent bool
			if sent, err = reporter.sendHistoryLocked(ctx); err == nil && sent {
				err = reporter.reportChangeLocked(ctx, "")
			}
			reporter.mu.Unlock()
		case <-refresh.C:
			err = reporter.reportChange(ctx, "")
		}
		if err != nil && ctx.Err() == nil {
			return err
		}
	}
}

// setInterval applies a TelemetryControl from the panel.
func (reporter *reporter) setInterval(interval time.Duration) {
	select {
	case <-reporter.interval:
	default:
	}
	reporter.interval <- interval
}

// reportChange sends the state report if it differs from the last one sent,
// naming the command that changed it, if any.
func (reporter *reporter) reportChange(ctx context.Context, requestID string) error {
	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	return reporter.reportChangeLocked(ctx, requestID)
}

func (reporter *reporter) reportChangeLocked(ctx context.Context, requestID string) error {
	report, err := reporter.stateReport(ctx)
	if err != nil {
		return err
	}
	encoded, err := encodeReport(report)
	if err != nil || bytes.Equal(encoded, reporter.lastReport) {
		return err
	}
	if err := reporter.sendEvent(&nodeapi.Event{RequestId: requestID, Body: &nodeapi.Event_StateReport{StateReport: report}}); err != nil {
		return err
	}
	reporter.lastReport = encoded
	return nil
}

// sendHistoryLocked sends the runtime transitions after historyAfter and
// reports whether there were any.
func (reporter *reporter) sendHistoryLocked(ctx context.Context) (bool, error) {
	sent := false
	for {
		transitions, err := reporter.database.RuntimeTransitionsAfter(ctx, reporter.historyAfter, historyBatchSize)
		if err != nil {
			return sent, err
		}
		for _, transition := range transitions {
			if err := reporter.sendEvent(&nodeapi.Event{Body: &nodeapi.Event_RuntimeTransition{RuntimeTransition: runtimeTransition(transition)}}); err != nil {
				return sent, err
			}
			reporter.historyAfter, sent = transition.ID, true
		}
		if len(transitions) < historyBatchSize {
			return sent, nil
		}
	}
}

func (reporter *reporter) sendMetrics(ctx context.Context) error {
	sample := &nodeapi.MetricsSample{SampledAt: timestamppb.Now()}
	if snapshot := reporter.sampler.Sample(reporter.dataDir); snapshot != nil {
		sample.SampledAt = timestamppb.New(snapshot.SampledAt)
		sample.CpuPercent, sample.CpuCount, sample.LoadOne = snapshot.CPUPercent, uint32(snapshot.CPUCount), snapshot.LoadOne
		sample.MemoryTotal, sample.MemoryUsed = snapshot.MemoryTotal, snapshot.MemoryUsed
		sample.DiskTotal, sample.DiskUsed = snapshot.DiskTotal, snapshot.DiskUsed
	}
	if traffic := reporter.node.LatestTraffic(); traffic != nil {
		sample.Traffic = &nodeapi.TrafficCounters{
			BundleId: traffic.ActivationBundleID, UploadBytes: uint64(traffic.UploadTotal), DownloadBytes: uint64(traffic.DownloadTotal),
			Connections: uint32(min(traffic.Connections, int64(^uint32(0)))), Pid: int64(traffic.PID),
			ProcessStartToken: traffic.ProcessStartToken, MemoryBytes: uint64(traffic.MemoryBytes),
		}
		sample.SampledAt = timestamppb.New(traffic.SampledAt)
	}
	if ctx.Err() != nil {
		return nil
	}
	return reporter.sendEvent(&nodeapi.Event{Body: &nodeapi.Event_MetricsSample{MetricsSample: sample}})
}

func (reporter *reporter) sendEvent(event *nodeapi.Event) error {
	return reporter.send(&nodeapi.ConnectRequest{Frame: &nodeapi.ConnectRequest_Event{Event: event}})
}

// stateReport reads the node's observed state.
func (reporter *reporter) stateReport(ctx context.Context) (*nodeapi.StateReport, error) {
	status, err := reporter.node.RuntimeStatus(ctx)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	report := &nodeapi.StateReport{
		Host:    &nodeapi.Host{Hostname: hostname, Platform: platform(), PublicIp: reporter.node.CachedPublicIP(ctx)},
		Runtime: runtimeOutcome(service.RuntimeResponse{Status: status}).GetStatus(),
	}
	hub, err := reporter.database.RuntimeHubState(ctx)
	if err != nil {
		return nil, err
	}
	if report.Applied, err = reporter.bundle(ctx, hub.AppliedBundleID, hub.AppliedAt); err != nil {
		return nil, err
	}
	if report.Rollback, err = reporter.bundle(ctx, hub.RollbackBundleID, nil); err != nil {
		return nil, err
	}
	filter := store.CoreArtifactListFilter{Limit: 200}
	for {
		page, err := reporter.database.ListCoreArtifacts(ctx, filter)
		if err != nil {
			return nil, err
		}
		for _, core := range page.Items {
			report.Installed = append(report.Installed, installedCore(core))
		}
		if page.Next == nil {
			return report, nil
		}
		filter.Cursor = page.Next
	}
}

func (reporter *reporter) bundle(ctx context.Context, bundleID string, appliedAt *time.Time) (*nodeapi.Bundle, error) {
	if bundleID == "" {
		return nil, nil
	}
	activation, err := reporter.database.GetActivationBundle(ctx, bundleID)
	if err != nil {
		return nil, err
	}
	startup, err := reporter.database.GetStartupArtifact(ctx, activation.StartupArtifactID)
	if err != nil {
		return nil, err
	}
	bundle := &nodeapi.Bundle{
		BundleId: activation.ID, StartupArtifactId: startup.ID, RevisionId: startup.CanonicalRevisionID,
		ConfigurationSha256: startup.ConfigSHA256, ArtifactId: startup.CoreArtifactID, ExactCoreVersion: startup.ExactCoreVersion,
		MonitoringTier: monitoringTierProto(activation.MonitoringTier),
	}
	if appliedAt != nil {
		bundle.AppliedAt = timestamppb.New(*appliedAt)
	}
	return bundle, nil
}

// appliedConfiguration returns the configuration the applied bundle was
// checked from.
func (reporter *reporter) appliedConfiguration(ctx context.Context, revisionID string) (*nodeapi.ConfigurationRevision, error) {
	revision, err := reporter.database.GetCanonicalRevision(ctx, revisionID)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(revision.Document)
	return &nodeapi.ConfigurationRevision{RevisionId: revision.ID, Sha256: hex.EncodeToString(sum[:]), Content: revision.Document}, nil
}

func encodeReport(report *nodeapi.StateReport) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(report)
}

func boundedInterval(interval time.Duration) time.Duration {
	return min(max(interval, minimumMetricsPeriod), maximumMetricsPeriod)
}

func runtimeTransition(transition store.RuntimeTransition) *nodeapi.RuntimeTransition {
	value := &nodeapi.RuntimeTransition{
		Id: transition.ID, Status: processState(transition.State), Reason: transition.Reason,
		BundleId: transition.ActivationBundleID, OccurredAt: timestamppb.New(transition.OccurredAt),
		Generation: transition.Generation, Pid: int64(transition.PID),
	}
	if transition.ProcessStartedAt != nil {
		value.ProcessStartedAt = timestamppb.New(*transition.ProcessStartedAt)
	}
	if transition.UncertainSince != nil {
		value.UncertainSince = timestamppb.New(*transition.UncertainSince)
	}
	return value
}

func processState(state store.RuntimeTransitionState) nodeapi.ProcessState {
	switch state {
	case store.RuntimeTransitionRunning:
		return nodeapi.ProcessState_PROCESS_STATE_RUNNING
	case store.RuntimeTransitionStopped:
		return nodeapi.ProcessState_PROCESS_STATE_STOPPED
	case store.RuntimeTransitionFailed:
		return nodeapi.ProcessState_PROCESS_STATE_FAILED
	case store.RuntimeTransitionUnknown:
		return nodeapi.ProcessState_PROCESS_STATE_UNKNOWN
	}
	return nodeapi.ProcessState_PROCESS_STATE_UNSPECIFIED
}
