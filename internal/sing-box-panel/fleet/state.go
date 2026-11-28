// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// errStateReportInvalid rejects a state report whose identifiers are too
// long or contain control characters.
var errStateReportInvalid = errors.New("the state report is malformed")

// LiveMetricsInterval is a node's metrics interval while a browser views its
// live metrics.
const LiveMetricsInterval = time.Second

// metricsStreamInterval is how often a node's live metrics view refreshes.
const metricsStreamInterval = time.Second

// receiveEvent applies one event from a node's session. A failure to apply
// one event is recorded and does not end the session, with one exception: it
// returns an error, which ends the session, when the session's first state
// report could not be recorded. Such a session would never accept commands,
// while the node's next session starts with a new full report.
func (service *Service) receiveEvent(ctx context.Context, current *session, event *nodeapi.Event) error {
	var err error
	code := ""
	opening := false
	switch body := event.GetBody().(type) {
	case *nodeapi.Event_StateReport:
		opening = !current.isReady()
		code, err = "node.state_report_failed", service.receiveStateReport(ctx, current, body.StateReport)
	case *nodeapi.Event_RuntimeTransition:
		code, err = "node.runtime_history_failed", service.records.RecordRuntimeTransition(ctx, runtimeTransitionInput(current, body.RuntimeTransition))
	case *nodeapi.Event_MetricsSample:
		code, err = "node.metrics_failed", service.records.RecordNodeSample(ctx, current.nodeID, current.instanceID, nodeSample(body.MetricsSample, service.now()))
	}
	if err == nil || ctx.Err() != nil {
		return nil
	}
	service.recordNodeEvent(store.LogLevelWarn, code, "A node report could not be recorded", current.nodeID)
	if opening {
		return nodeapi.Errorf(nodeapi.CodeUnavailable, "the panel could not record the session's first state report")
	}
	return nil
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
	summary := store.NodeReport{PublicIP: report.GetHost().GetPublicIp()}
	if applied := report.GetApplied(); applied != nil {
		summary.Applied = store.AppliedBundle{
			ID: applied.GetBundleId(), RevisionID: applied.GetRevisionId(),
			ConfigSHA256: applied.GetConfigurationSha256(), ExactCoreVersion: applied.GetExactCoreVersion(),
		}
	}
	if !boundedText(summary.PublicIP, 64) || !boundedText(summary.Applied.ID, 256) || !boundedText(summary.Applied.RevisionID, 256) ||
		!boundedText(summary.Applied.ConfigSHA256, 64) || !boundedText(summary.Applied.ExactCoreVersion, 64) {
		return errStateReportInvalid
	}
	published, err := service.database.RecordNodeStateReport(ctx, current.nodeID, current.sessionID, encoded, summary, service.now().UTC())
	if err != nil {
		return err
	}
	if published {
		service.publicationChanged(ctx)
	}
	if existing != nil {
		service.seedConfiguration(ctx, current.nodeID, existing)
	}
	if last := report.GetLastUpdate(); last != nil && !proto.Equal(last, current.lastUpdate) {
		current.lastUpdate = last
		service.recordUpdateReport(current.nodeID, last)
	}
	current.open()
	return nil
}

// seedConfiguration saves the configuration a node already runs as its
// editable file, under the node's revision ID, while that file is still the
// empty document the node was created with. Saving at that revision fails
// harmlessly if it was edited.
func (service *Service) seedConfiguration(ctx context.Context, nodeID string, existing *nodeapi.ConfigurationRevision) {
	file, err := service.configs.ConfigurationFile(ctx, nodeID)
	if err != nil || file.Revision != 1 || file.Content != string(configuration.Empty().CanonicalJSON()) {
		return
	}
	if _, err := service.configs.SeedConfigurationFile(ctx, nodeID, file.Revision,
		existing.GetRevisionId(), string(existing.GetContent())); err == nil {
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

// StatusStreamInterval is how often the shared fleet snapshot is read again
// while anyone streams it, and the oldest snapshot Status reuses.
const StatusStreamInterval = 2 * time.Second

// NodeStatus is one node in the fleet views with its live evidence and
// recorded traffic, without the newest stored sample and the period's
// counters that only the node's own metrics carry.
type NodeStatus struct {
	Node    Node                      `json:"node"`
	Metrics telemetry.MetricsSnapshot `json:"metrics"`
}

// FleetSnapshot is every node with its metrics, read at one time, in name
// order, with each node's JSON encoding, made once for every reader.
type FleetSnapshot struct {
	Nodes   []NodeStatus
	Encoded []json.RawMessage
}

// Status returns the fleet snapshot: the shared one while it is at most
// StatusStreamInterval old, or else a new read that later readers share.
func (service *Service) Status(ctx context.Context) (FleetSnapshot, error) {
	return service.statusFeed.Read(ctx, StatusStreamInterval)
}

// expireStatus makes the next fleet read see a write the panel answered,
// such as a renamed node or the state a command left. Node reports, which
// arrive constantly, reach readers with the next shared read instead.
func (service *Service) expireStatus() {
	service.statusFeed.Expire()
}

// SubscribeStatus streams the fleet snapshot every StatusStreamInterval.
// One read serves every subscriber, and none runs without them.
func (service *Service) SubscribeStatus() (<-chan telemetry.SnapshotResult[FleetSnapshot], func()) {
	return service.statusFeed.Subscribe()
}

// statusEncoding is one node's item in a fleet snapshot and its encoding.
type statusEncoding struct {
	status  NodeStatus
	encoded json.RawMessage
}

// collectStatus reads every node with its metrics: one query for the
// nodes, and each node's live evidence and recorded traffic from memory.
func (service *Service) collectStatus(ctx context.Context) (FleetSnapshot, error) {
	records, err := service.database.Nodes(ctx)
	if err != nil {
		return FleetSnapshot{}, err
	}
	service.statusMu.Lock()
	previous := service.statusItems
	service.statusMu.Unlock()
	release := service.releaseVersion()
	items := make(map[string]statusEncoding, len(records))
	snapshot := FleetSnapshot{Nodes: make([]NodeStatus, 0, len(records)), Encoded: make([]json.RawMessage, 0, len(records))}
	for _, record := range records {
		state := nodeState(record)
		metrics, err := service.recordSnapshot(ctx, record, state)
		if err != nil {
			return FleetSnapshot{}, err
		}
		status := NodeStatus{Node: nodeView(record, state, release), Metrics: metrics.Metrics}
		status.Metrics.LatestSample, status.Metrics.CurrentTrafficData.Counters = nil, nil
		last, known := previous[record.ID]
		item, err := encodeStatus(status, last, known)
		if err != nil {
			return FleetSnapshot{}, err
		}
		items[record.ID] = item
		snapshot.Nodes = append(snapshot.Nodes, item.status)
		snapshot.Encoded = append(snapshot.Encoded, item.encoded)
	}
	service.statusMu.Lock()
	service.statusItems = items
	service.statusMu.Unlock()
	return snapshot, nil
}

// encodeStatus encodes a node's item. While it differs from the node's
// previous item only in when its metrics were read, the previous item and
// encoding stay, so an unchanged node encodes to the same bytes in every
// snapshot: streams send only changes, and an unchanged page keeps its ETag.
func encodeStatus(status NodeStatus, previous statusEncoding, known bool) (statusEncoding, error) {
	if known {
		unchanged := status
		unchanged.Metrics.CollectedAt = previous.status.Metrics.CollectedAt
		encoded, err := json.Marshal(unchanged)
		if err != nil {
			return statusEncoding{}, err
		}
		if bytes.Equal(encoded, previous.encoded) {
			return previous, nil
		}
	}
	encoded, err := json.Marshal(status)
	return statusEncoding{status: status, encoded: encoded}, err
}

func (service *Service) metricsStreamSnapshot(ctx context.Context, nodeID string) (MetricsStreamSnapshot, error) {
	record, err := service.database.Node(ctx, nodeID)
	if err != nil {
		return MetricsStreamSnapshot{}, err
	}
	return service.recordSnapshot(ctx, record, nodeState(record))
}

// recordSnapshot returns the metrics of the node in record, whose last
// report decodes to state.
func (service *Service) recordSnapshot(ctx context.Context, record store.Node, state *NodeState) (MetricsStreamSnapshot, error) {
	var applied *telemetry.AppliedBundle
	var runtime RuntimeStatus
	if state != nil {
		runtime = runtimeStatusView(state.Report.GetRuntime())
		if bundle := state.Report.GetApplied(); bundle != nil {
			applied = &telemetry.AppliedBundle{ID: bundle.GetBundleId(), MonitoringTier: monitoringTierView(bundle.GetMonitoringTier())}
		}
	}
	metrics, err := service.records.NodeMetrics(ctx, record, applied)
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
	service.viewersMu.Unlock()
	service.syncMetricsInterval(nodeID)
	snapshots, unsubscribe := feed.Subscribe()
	var once sync.Once
	return snapshots, func() {
		once.Do(func() {
			unsubscribe()
			service.viewersMu.Lock()
			service.viewers[nodeID]--
			if service.viewers[nodeID] == 0 {
				delete(service.viewers, nodeID)
				// The feed has no subscribers left and stopped reading.
				delete(service.metricsFeeds, nodeID)
			}
			service.viewersMu.Unlock()
			service.syncMetricsInterval(nodeID)
		})
	}
}

// syncMetricsInterval tells the node's session, if any, how often to sample:
// every LiveMetricsInterval while anyone views its live metrics, else every
// MetricsInterval. Each session applies changes one at a time and reads the
// viewer count only once it holds its turn, so the node ends with the
// interval of the newest count; no fleet-wide lock is held while sending.
func (service *Service) syncMetricsInterval(nodeID string) {
	current := service.sessions.get(nodeID)
	if current == nil {
		return
	}
	current.telemetryMu.Lock()
	defer current.telemetryMu.Unlock()
	if !current.welcomed {
		// The session syncs once it has sent Welcome.
		return
	}
	service.viewersMu.Lock()
	interval := MetricsInterval
	if service.viewers[nodeID] > 0 {
		interval = LiveMetricsInterval
	}
	service.viewersMu.Unlock()
	if current.metricsInterval == interval {
		return
	}
	if err := current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_TelemetryControl{
		TelemetryControl: &nodeapi.TelemetryControl{MetricsInterval: durationpb.New(interval)},
	}}); err == nil {
		current.metricsInterval = interval
	}
}

// coreLogPolicyMinor is the protocol minor version from which a node takes
// the core log policy.
const coreLogPolicyMinor = 3

// coreLogPolicy is the core log retention the panel's settings hold, as the
// protocol carries it.
func (service *Service) coreLogPolicy(ctx context.Context) (*nodeapi.CoreLogPolicy, error) {
	value, _, err := service.settings.Current(ctx)
	if err != nil {
		return nil, err
	}
	return &nodeapi.CoreLogPolicy{
		RetentionDays: uint32(value.Logs.CoreRetentionDays), MaxFiles: uint32(value.Logs.CoreMaxFiles),
		MaxFileBytes: uint64(value.Logs.CoreMaxFileSizeMiB) << 20,
	}, nil
}

// SyncCoreLogPolicy sends every connected node that takes it the core log
// retention the panel's settings hold, when it differs from what the node was
// last told. The server calls it after each settings change; a node that
// connects later receives the policy in Welcome.
func (service *Service) SyncCoreLogPolicy(ctx context.Context) error {
	policy, err := service.coreLogPolicy(ctx)
	if err != nil {
		return err
	}
	for _, current := range service.sessions.all() {
		current.telemetryMu.Lock()
		if current.welcomed && current.protocolMinor >= coreLogPolicyMinor && !proto.Equal(current.coreLogPolicy, policy) {
			if err := current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_CoreLogPolicy{
				CoreLogPolicy: policy,
			}}); err == nil {
				current.coreLogPolicy = policy
			}
		}
		current.telemetryMu.Unlock()
	}
	return nil
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
// maximumSampleSkew is how far a sample's time may lie behind the panel's
// clock before the panel's own time replaces it. A sample that old still
// counts as live until the next baseline sample arrives.
const maximumSampleSkew = 20 * time.Second

// nodeSample converts a metrics sample received at now. Samples are sent
// live, so a time from the node's clock that lies ahead of the panel's or
// more than maximumSampleSkew behind it is replaced with now: a node whose
// clock is wrong never stores a time that later samples cannot pass.
func nodeSample(sample *nodeapi.MetricsSample, now time.Time) telemetry.NodeSample {
	sampledAt := sample.GetSampledAt().AsTime()
	if now = now.UTC(); sampledAt.After(now) || now.Sub(sampledAt) > maximumSampleSkew {
		sampledAt = now
	}
	result := telemetry.NodeSample{SampledAt: sampledAt}
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
