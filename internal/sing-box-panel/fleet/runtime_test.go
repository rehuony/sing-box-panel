// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"errors"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// TestStartRunsAnEnabledCoreThePanelHasNoRecordOf starts a node whose
// enabled core is missing from the panel's cache, as after the panel's data
// was recreated: the start goes ahead for the node's own check, while
// enabling such a core is refused by the panel.
func TestStartRunsAnEnabledCoreThePanelHasNoRecordOf(t *testing.T) {
	served := startFleet(t, nil)
	node := enrolledRawNode(t, served, "edge", nodeapi.ProtocolMinor, "test")
	stream, err := node.connect(t, node.key)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, served.service, node.id, fleet.StateOnline)
	file, err := served.configs.ConfigurationFile(t.Context(), node.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := served.configs.SaveConfigurationFile(t.Context(), node.id, nodeconfig.ConfigurationFileWrite{Revision: file.Revision, Content: `{"log":{"level":"warn"}}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := served.service.EnableCore(t.Context(), node.id, "core_unknown"); !errors.Is(err, store.ErrCachedArtifactNotFound) {
		t.Fatalf("enable of an uncached core = %v, want ErrCachedArtifactNotFound", err)
	}

	started := make(chan error, 1)
	go func() {
		_, err := served.service.StartRuntime(t.Context(), node.id)
		started <- err
	}()
	frame, err := stream.Recv()
	if err != nil || frame.GetCommand().GetGetRuntimeStatus() == nil {
		t.Fatalf("expected a status command first, got %v, %v", frame, err)
	}
	answerCommand(t, stream, frame.GetCommand().GetRequestId(), &nodeapi.Result_Runtime{Runtime: &nodeapi.RuntimeOutcome{Status: &nodeapi.RuntimeStatus{
		EnabledCoreArtifactId: "core_unknown", EnabledExactCoreVersion: "1.14.4",
	}}})
	received := make(chan *nodeapi.ConnectResponse, 1)
	go func() {
		frame, _ := stream.Recv()
		received <- frame
	}()
	select {
	case err := <-started:
		t.Fatalf("start ended before it reached the node: %v", err)
	case frame = <-received:
	}
	if frame.GetCommand().GetStartRuntime().GetConfiguration() == nil {
		t.Fatalf("expected a start command with the configuration, got %v", frame)
	}
	answerCommand(t, stream, frame.GetCommand().GetRequestId(), &nodeapi.Result_Runtime{Runtime: &nodeapi.RuntimeOutcome{Status: &nodeapi.RuntimeStatus{}}})
	if err := <-started; err != nil {
		t.Fatal(err)
	}
}
