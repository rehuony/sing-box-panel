// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticsNeverContainOutboundValues(t *testing.T) {
	t.Parallel()
	const secret = "credential-that-must-not-leak"
	startup := []byte(`{"outbounds":[{"type":"vmess","tag":"sensitive-tag","server":"sensitive.example","server_port":443,"uuid":"` + secret + `","transport":{"type":"grpc"}}]}`)
	result, err := Render(startup, RenderChannel{Format: RenderFormatLoon})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	encoded, err := json.Marshal(result.Diagnostics)
	if err != nil {
		t.Fatalf("Marshal diagnostics: %v", err)
	}
	for _, sensitive := range []string{secret, "sensitive-tag", "sensitive.example"} {
		if strings.Contains(string(encoded), sensitive) {
			t.Fatalf("diagnostics leaked %q: %s", sensitive, encoded)
		}
	}
}

func TestPreviewDiagnosticsFollowSelectedNodeIdentity(t *testing.T) {
	bad := testChannelNodes(t, "bad-source", `{"outbounds":[{"type":"anytls","tag":"Shared","server":"sensitive.example","server_port":443,"password":"secret-value","tls":{"enabled":true,"fragment":true}}]}`)
	good := testChannelNodes(t, "good-source", `{"outbounds":[{"type":"socks","tag":"Shared","server":"good.example","server_port":1080}]}`)
	excluded := testChannelNodes(t, "excluded-source", `{"outbounds":[{"type":"anytls","tag":"Excluded","server":"excluded.example","server_port":443,"password":"secret","tls":{"enabled":true,"fragment":true}}]}`)
	good[0].Name = "Shared 2"
	badID, goodID := bad[0].ID, good[0].ID
	for _, nodes := range [][]ChannelNode{{excluded[0], good[0], bad[0]}, {excluded[0], bad[0], good[0]}} {
		config := ChannelConfig{IncompatibleNodes: "skip", DefaultExit: RouteExit{Kind: "direct"}, Groups: []RuleGroup{
			{ID: "main", Name: "Main", Enabled: true, Type: "select", Members: nodeMembers(goodID, badID)},
		}}
		result, err := RenderChannelNodes(nodes, RenderFormatLoon, config, nil)
		if err != nil || result.NodeCount != 1 || len(result.PreviewDiagnostics) != 1 {
			t.Fatalf("render: %v, %+v", err, result.PreviewDiagnostics)
		}
		issue := result.PreviewDiagnostics[0]
		position := 0
		if nodes[1].ID == goodID {
			position = 1
		}
		if issue.NodeID != badID || issue.NodeName != "Shared" || issue.NodeType != "anytls" || issue.Reason != "unsupported_tls_option" || !strings.HasSuffix(issue.FieldPath, ".tls.fragment") || issue.ItemIndex != position {
			t.Fatalf("diagnostic points to wrong node: %+v", issue)
		}
		public, _ := json.Marshal(result.Diagnostics)
		preview, _ := json.Marshal(result.PreviewDiagnostics)
		for _, value := range []string{"sensitive.example", "secret-value"} {
			if strings.Contains(string(public), value) || strings.Contains(string(preview), value) {
				t.Fatal("diagnostic reflected a configuration value")
			}
		}
		if strings.Contains(string(public), "Shared") || strings.Contains(string(public), badID) {
			t.Fatal("public diagnostics included preview identity")
		}
	}
}
