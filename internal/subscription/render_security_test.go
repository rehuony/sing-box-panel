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
	bad, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"anytls","tag":"Shared","server":"sensitive.example","server_port":443,"password":"secret-value","tls":{"enabled":true,"fragment":true}}]}`), "bad-source")
	if err != nil {
		t.Fatal(err)
	}
	good, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"socks","tag":"Shared","server":"good.example","server_port":1080}]}`), "good-source")
	if err != nil {
		t.Fatal(err)
	}
	excluded, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"anytls","tag":"Excluded","server":"excluded.example","server_port":443,"password":"secret","tls":{"enabled":true,"fragment":true}}]}`), "excluded-source")
	if err != nil {
		t.Fatal(err)
	}
	nodes := append(append(excluded, bad...), good...)
	badID, goodID := PublicationID(bad[0]), PublicationID(good[0])
	for _, ids := range [][]string{{goodID, badID}, {badID, goodID}} {
		policy := &ChannelPolicy{Selection: NodeSelection{IDs: ids, NewNodePolicy: "exclude"}, IncompatibleNodes: "skip", DefaultExit: RouteExit{Kind: "direct"}, Groups: []RuleGroup{}}
		result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatLoon}, policy)
		if err != nil || result.NodeCount != 1 || len(result.PreviewDiagnostics) != 1 {
			t.Fatalf("render: %v, %+v", err, result.PreviewDiagnostics)
		}
		issue := result.PreviewDiagnostics[0]
		if issue.NodeID != badID || !strings.HasPrefix(issue.NodeName, "Shared") || issue.NodeType != "anytls" || issue.Reason != "unsupported_tls_option" || !strings.HasSuffix(issue.FieldPath, ".tls.fragment") || ids[issue.ItemIndex] != badID {
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
	// Legacy publication uses key ordering before exclusions, not catalog order.
	result, err := RenderPolicyNodes(append(bad, excluded...), RenderChannel{Format: RenderFormatLoon, ExcludeTags: []string{"Excluded"}}, nil)
	if err != nil || len(result.PreviewDiagnostics) != 1 || result.PreviewDiagnostics[0].NodeID != badID || result.PreviewDiagnostics[0].NodeName != "Shared" {
		t.Fatalf("legacy identity mismatch: %+v, %v", result.PreviewDiagnostics, err)
	}
}
