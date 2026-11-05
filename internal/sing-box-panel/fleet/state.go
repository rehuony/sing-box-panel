// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// LiveMetricsInterval is a node's metrics interval while a browser views its
// live metrics.
const LiveMetricsInterval = time.Second

// metricsStreamInterval is how often a node's live metrics view refreshes.
const metricsStreamInterval = time.Second

// receiveEvent applies one event from a node's session. A failure to apply
// one event is recorded and does not end the session.
func (service *Service) receiveEvent(ctx context.Context, current *session, event *nodeapi.Event) {
	var err error
	code := ""
	switch body := event.GetBody().(type) {
	case *nodeapi.Event_StateReport:
		code, err = "node.state_report_failed", service.receiveStateReport(ctx, current, body.StateReport)
	case *nodeapi.Event_RuntimeTransition:
		code, err = "node.runtime_history_failed", service.records.RecordRuntimeTransition(ctx, runtimeTransitionInput(current, body.RuntimeTransition))
	case *nodeapi.Event_MetricsSample:
		code, err = "node.metrics_failed", service.records.RecordNodeSample(ctx, current.nodeID, current.instanceID, nodeSample(body.MetricsSample))
	}
	if err != nil && ctx.Err() == nil {
		service.recordNodeEvent(store.LogLevelWarn, code, "A node report could not be recorded", current.nodeID)
	}
}

// receiveStateReport stores the node's observed state. The first report of a
// session opens it to commands, and may seed the node's configuration.
func (service *Service) receiveStateReport(ctx context.Context, current *session, report *nodeapi.StateReport) error {
	existing := report.GetExistingConfiguration()
	stored := proto.Clone(report).(*nodeapi.StateReport)
	stored.ExistingConfiguration = nil
	encoded, err := proto.Marshal(stored)
	if err != nil {
		return err
	}
	if err := service.database.RecordNodeStateReport(ctx, current.nodeID, current.sessionID, encoded, service.now().UTC()); err != nil {
		return err
	}
	if existing != nil {
		service.seedConfiguration(ctx, current.nodeID, existing)
	}
	current.open()
	return nil
}

// seedConfiguration saves the configuration a node already runs as its
// editable file while that file is still the empty document the node was
// created with. Saving at that revision fails harmlessly if it was edited.
func (service *Service) seedConfiguration(ctx context.Context, nodeID string, existing *nodeapi.ConfigurationRevision) {
	file, err := service.configs.ConfigurationFile(ctx, nodeID)
	if err != nil || file.Revision != 1 || file.Content != string(configuration.Empty().CanonicalJSON()) {
		return
	}
	if _, err := service.configs.SaveConfigurationFile(ctx, nodeID, nodeconfig.ConfigurationFileWrite{
		Revision: file.Revision, Content: string(existing.GetContent()),
	}); err == nil {
		service.recordNodeEvent(store.LogLevelInfo, "node.configuration_seeded", "Node configuration taken from the configuration it runs", nodeID)
	}
}

// NodeState is what a node last reported about itself.
type NodeState struct {
	Report     *nodeapi.StateReport
	ReportedAt time.Time
}

// nodeState decodes the state the node last reported, or nil before its
// first report.
func nodeState(record store.Node) *NodeState {
	if record.StateReport == nil || record.ReportedAt == nil {
		return nil
	}
	var report nodeapi.StateReport
	if proto.Unmarshal(record.StateReport, &report) != nil {
		return nil
	}
	return &NodeState{Report: &report, ReportedAt: *record.ReportedAt}
}

// MetricsStreamSnapshot is one frame of a node's live metrics view.
type MetricsStreamSnapshot struct {
	Metrics telemetry.MetricsSnapshot `json:"metrics"`
	Runtime RuntimeStatus             `json:"runtime"`
}

// NodeMetrics returns a node's live evidence and recorded traffic.
func (service *Service) NodeMetrics(ctx context.Context, nodeID string) (telemetry.MetricsSnapshot, error) {
	snapshot, err := service.metricsStreamSnapshot(ctx, nodeID)
	return snapshot.Metrics, err
}

func (service *Service) metricsStreamSnapshot(ctx context.Context, nodeID string) (MetricsStreamSnapshot, error) {
	record, err := service.database.Node(ctx, nodeID)
	if err != nil {
		return MetricsStreamSnapshot{}, err
	}
	var applied *telemetry.AppliedBundle
	var runtime RuntimeStatus
	if state := nodeState(record); state != nil {
		runtime = runtimeStatusView(state.Report.GetRuntime())
		if bundle := state.Report.GetApplied(); bundle != nil {
			applied = &telemetry.AppliedBundle{ID: bundle.GetBundleId(), MonitoringTier: monitoringTierView(bundle.GetMonitoringTier())}
		}
	}
	metrics, err := service.records.NodeMetrics(ctx, nodeID, applied)
	if err != nil {
		return MetricsStreamSnapshot{}, err
	}
	// Live traffic belongs to the process the node reports running.
	if live := metrics.LiveSample; live != nil {
		running := runtime.Running
		if running == nil || runtime.ObservationState != "running" || running.PID != live.PID ||
			running.ProcessStartToken != live.ProcessStartToken || running.ActivationBundleID != live.ActivationBundleID {
			metrics.LiveSample = nil
		}
	}
	return MetricsStreamSnapshot{Metrics: metrics, Runtime: runtime}, nil
}

// SubscribeMetrics streams a node's live metrics view. While anyone views
// it, the node samples once a second.
func (service *Service) SubscribeMetrics(nodeID string) (<-chan telemetry.SnapshotResult[MetricsStreamSnapshot], func()) {
	service.viewersMu.Lock()
	feed := service.metricsFeeds[nodeID]
	if feed == nil {
		feed = telemetry.NewFeed(metricsStreamInterval, func(ctx context.Context) (MetricsStreamSnapshot, error) {
			return service.metricsStreamSnapshot(ctx, nodeID)
		})
		service.metricsFeeds[nodeID] = feed
	}
	service.viewers[nodeID]++
	if service.viewers[nodeID] == 1 {
		service.setMetricsInterval(nodeID, LiveMetricsInterval)
	}
	service.viewersMu.Unlock()
	snapshots, unsubscribe := feed.Subscribe()
	var once sync.Once
	return snapshots, func() {
		once.Do(func() {
			unsubscribe()
			service.viewersMu.Lock()
			defer service.viewersMu.Unlock()
			service.viewers[nodeID]--
			if service.viewers[nodeID] == 0 {
				delete(service.viewers, nodeID)
				service.setMetricsInterval(nodeID, MetricsInterval)
			}
		})
	}
}

// setMetricsInterval tells the node's session, if any, how often to sample.
// The caller holds viewersMu, so the node receives changes in order.
func (service *Service) setMetricsInterval(nodeID string, interval time.Duration) {
	if current := service.sessions.get(nodeID); current != nil {
		_ = current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_TelemetryControl{
			TelemetryControl: &nodeapi.TelemetryControl{MetricsInterval: durationpb.New(interval)},
		}})
	}
}

func runtimeTransitionInput(current *session, transition *nodeapi.RuntimeTransition) store.RuntimeTransitionInput {
	return store.RuntimeTransitionInput{
		NodeID: current.nodeID, InstanceID: current.instanceID, SourceID: transition.GetId(),
		State: transitionState(transition.GetStatus()), Reason: transition.GetReason(),
		ActivationBundleID: transition.GetBundleId(), Generation: transition.GetGeneration(), PID: int(transition.GetPid()),
		ProcessStartedAt: optionalTime(transition.GetProcessStartedAt()), OccurredAt: transition.GetOccurredAt().AsTime(),
		UncertainSince: optionalTime(transition.GetUncertainSince()),
	}
}

func transitionState(state nodeapi.ProcessState) store.RuntimeTransitionState {
	switch state {
	case nodeapi.ProcessState_PROCESS_STATE_RUNNING:
		return store.RuntimeTransitionRunning
	case nodeapi.ProcessState_PROCESS_STATE_STOPPED:
		return store.RuntimeTransitionStopped
	case nodeapi.ProcessState_PROCESS_STATE_FAILED:
		return store.RuntimeTransitionFailed
	case nodeapi.ProcessState_PROCESS_STATE_UNKNOWN:
		return store.RuntimeTransitionUnknown
	}
	return ""
}

func optionalTime(value *timestamppb.Timestamp) *time.Time {
	if value == nil {
		return nil
	}
	parsed := value.AsTime()
	return &parsed
}

// nodeSample maps a node's metrics sample, dropping counters too large to
// be real.
func nodeSample(sample *nodeapi.MetricsSample) telemetry.NodeSample {
	result := telemetry.NodeSample{SampledAt: sample.GetSampledAt().AsTime()}
	if sample.GetCpuCount() > 0 {
		result.Host = &telemetry.HostMetrics{
			SampledAt: result.SampledAt, CPUPercent: sample.CpuPercent, CPUCount: int(sample.GetCpuCount()), LoadOne: sample.LoadOne,
			MemoryTotal: sample.MemoryTotal, MemoryUsed: sample.MemoryUsed, DiskTotal: sample.DiskTotal, DiskUsed: sample.DiskUsed,
		}
	}
	if traffic := sample.GetTraffic(); traffic != nil && traffic.GetUploadBytes() <= maximumCounter &&
		traffic.GetDownloadBytes() <= maximumCounter && traffic.GetMemoryBytes() <= maximumCounter {
		result.Traffic = &telemetry.NodeTraffic{
			ActivationBundleID: traffic.GetBundleId(), PID: int(traffic.GetPid()), ProcessStartToken: traffic.GetProcessStartToken(),
			UploadTotal: int64(traffic.GetUploadBytes()), DownloadTotal: int64(traffic.GetDownloadBytes()),
			Connections: int64(traffic.GetConnections()), MemoryBytes: int64(traffic.GetMemoryBytes()),
		}
	}
	return result
}

// maximumCounter is the largest byte count stored as a signed integer.
const maximumCounter = 1<<63 - 1
