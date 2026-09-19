// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func policyFixture(t *testing.T, format RenderFormat) ([]Node, *ChannelPolicy) {
	t.Helper()
	nodes, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"socks","tag":"Tokyo","server":"tokyo.example.com","server_port":1080},{"type":"socks","tag":"Hong Kong","server":"hk.example.com","server_port":1080}]}`), "source-one")
	if err != nil {
		t.Fatal(err)
	}
	a, b := PublicationID(nodes[0]), PublicationID(nodes[1])
	p := &ChannelPolicy{Selection: NodeSelection{IDs: []string{a, b}, NewNodePolicy: "exclude"}, Organizer: NodeOrganizer{Sort: "none", Incompatible: "error"}, DefaultExit: RouteExit{Kind: "group", ID: "main"}, Groups: []RuleGroup{{ID: "main", Name: "Selected", Enabled: true, NodeIDs: []string{a, b}, DefaultExit: RouteExit{Kind: "node", ID: a}, Rules: []ChannelRule{
		{ID: "domain", Enabled: true, Kind: "domain_suffix", Value: "example.org", Exit: RouteExit{Kind: "group-default"}},
		{ID: "ipv6", Enabled: true, Kind: "ip_cidr", Value: "2001:db8::/32", Exit: RouteExit{Kind: "direct"}},
		{ID: "disabled", Enabled: false, Kind: "domain", Value: "disabled.example", Exit: RouteExit{Kind: "reject"}},
		{ID: "remote", Enabled: true, Kind: "remote", Exit: RouteExit{Kind: "node", ID: b}, Remote: &RemoteRuleSet{Name: "Remote rules", URL: "https://raw.githubusercontent.com/example/rules/main/proxy.rules", Format: "source", Accelerated: true, UpdateInterval: 3600}},
	}}}}
	if format == RenderFormatMihomo {
		p.Groups[0].Rules[3].Remote.Format = "mrs"
		p.Groups[0].Rules[3].Remote.Behavior = "domain"
	}
	return nodes, p
}

func TestChannelNativeRenderingAndTemplatePreservation(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo} {
		t.Run(string(format), func(t *testing.T) {
			nodes, p := policyFixture(t, format)
			p.Template = &NativeTemplate{Format: format, Content: `{"log":{"level":"warn"},"custom_counter":9007199254740993,"route":{"auto_detect_interface":true}}`}
			if format == RenderFormatMihomo {
				p.Template.Content = "# retained comment\nlog-level: warning\ncustom-counter: 9007199254740993\ndns:\n  enable: true\n"
			}
			result, err := RenderPolicyNodes(nodes, RenderChannel{Format: format}, p)
			if err != nil {
				t.Fatal(err)
			}
			if result.NodeCount != 2 || !bytes.Contains(result.Content, []byte("https://gh-proxy.com/https://raw.githubusercontent.com/")) || !bytes.Contains(result.Content, []byte("9007199254740993")) || bytes.Contains(result.Content, []byte("disabled.example")) {
				t.Fatal(string(result.Content))
			}
			again, err := RenderPolicyNodes(nodes, RenderChannel{Format: format}, p)
			if err != nil || !bytes.Equal(result.Content, again.Content) {
				t.Fatal("unstable or mutated render", err)
			}
			var root map[string]any
			if format == RenderFormatSingBox {
				root, err = DecodeDocumentObject(result.Content)
				if err != nil {
					t.Fatal(err)
				}
				route := root["route"].(map[string]any)
				if route["auto_detect_interface"] != true || route["final"] != "Selected" {
					t.Fatal(route)
				}
				rules := route["rules"].([]any)
				if len(rules) != 3 || rules[0].(map[string]any)["outbound"] != "Selected" || rules[2].(map[string]any)["outbound"] != nodes[1].Tag {
					t.Fatal(rules)
				}
				set := route["rule_set"].([]any)[0].(map[string]any)
				if set["format"] != "source" || set["download_detour"] != "direct" {
					t.Fatal(set)
				}
			} else {
				if err := yaml.Unmarshal(result.Content, &root); err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(result.Content, []byte("# retained comment")) {
					t.Fatal("template comment lost")
				}
				rules := root["rules"].([]any)
				if len(rules) != 4 || rules[1] != "IP-CIDR6,2001:db8::/32,DIRECT" || rules[3] != "MATCH,Selected" {
					t.Fatal(rules)
				}
				provider := root["rule-providers"].(map[string]any)["rules-main-remote"].(map[string]any)
				if provider["format"] != "mrs" || provider["behavior"] != "domain" {
					t.Fatal(provider)
				}
			}
		})
	}
}

func TestChannelUnavailableNodesNeverLeakOrFallbackDirect(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatSingBox)
	// Caller has filtered the first node after it was hidden. Membership remains
	// unchanged, and its group/default route fails closed until it is restored.
	result, err := RenderPolicyNodes(nodes[1:], RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != 1 || bytes.Contains(result.Content, []byte("tokyo.example.com")) {
		t.Fatal(string(result.Content))
	}
	root, _ := DecodeDocumentObject(result.Content)
	rules := root["route"].(map[string]any)["rules"].([]any)
	if rules[0].(map[string]any)["action"] != "reject" || rules[len(rules)-1].(map[string]any)["action"] != "reject" {
		t.Fatal(rules)
	}
	if len(p.Selection.IDs) != 2 || len(p.Groups[0].NodeIDs) != 2 {
		t.Fatal("render mutated membership")
	}
	restored, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil || restored.NodeCount != 2 {
		t.Fatal(restored, err)
	}
}

func TestChannelSelectionOrganizerAndFixedGroupCandidates(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatSingBox)
	more, _, _ := ParseSource(SourceFormatSingBoxJSON, []byte(`{"type":"socks","tag":"Amsterdam","server":"ams.example.com","server_port":1080}`), "new-source")
	nodes = append(nodes, more...)
	p.Selection.NewNodePolicy = "include"
	p.Organizer.Prefix = "Travel · "
	p.Organizer.Sort = "name"
	result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := DecodeDocumentObject(result.Content)
	out := root["outbounds"].([]any)
	if out[0].(map[string]any)["tag"] != "Travel · Amsterdam" {
		t.Fatal(out)
	}
	group := out[len(out)-1].(map[string]any)
	encoded, _ := json.Marshal(group)
	if bytes.Contains(encoded, []byte("Amsterdam")) {
		t.Fatal("new node entered an existing group")
	}
	if result.NodeCount != 3 {
		t.Fatal(result.NodeCount)
	}
	p.Selection.NewNodePolicy = "exclude"
	result, err = RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil || result.NodeCount != 2 {
		t.Fatal(result, err)
	}
}

func TestInvalidChannelPoliciesExposePathsWithoutInput(t *testing.T) {
	for name, change := range map[string]func(*ChannelPolicy){
		"native format":      func(p *ChannelPolicy) { p.Groups[0].Rules[3].Remote.Format = "mrs" },
		"node outside group": func(p *ChannelPolicy) { p.Groups[0].Rules[0].Exit = RouteExit{Kind: "node", ID: "not-selected"} },
		"template conflict": func(p *ChannelPolicy) {
			p.Template = &NativeTemplate{Format: RenderFormatSingBox, Content: `{"route":{"rules":[{"password":"sensitive-content"}]}}`}
		},
		"template duplicate key": func(p *ChannelPolicy) {
			p.Template = &NativeTemplate{Format: RenderFormatSingBox, Content: `{"log":{},"log":{}}`}
		},
		"disabled fallback": func(p *ChannelPolicy) { p.Groups[0].Enabled = false },
		"invalid CIDR":      func(p *ChannelPolicy) { p.Groups[0].Rules[1].Value = "not-an-ip" },
		"URL credentials":   func(p *ChannelPolicy) { p.Groups[0].Rules[3].Remote.URL = "https://secret:credential@example.com/path" },
	} {
		t.Run(name, func(t *testing.T) {
			_, p := policyFixture(t, RenderFormatSingBox)
			change(p)
			err := ValidateChannelPolicy(p, RenderFormatSingBox)
			var e *PolicyError
			if !errors.As(err, &e) || !strings.HasPrefix(e.Path, "policy") || strings.Contains(err.Error(), "sensitive-content") || strings.Contains(err.Error(), "credential@example") {
				t.Fatal(err)
			}
		})
	}
	_, p := policyFixture(t, RenderFormatMihomo)
	p.Groups[0].Rules[3].Remote.Behavior = "classical"
	if err := ValidateChannelPolicy(p, RenderFormatMihomo); err == nil {
		t.Fatal("MRS classical accepted")
	}
}

func TestRuleURLAccelerationIsReversibleAndDeduplicated(t *testing.T) {
	direct := "https://raw.githubusercontent.com/example/rules/main/file%20name?ref=abc%2Fdef"
	wrapped := "https://ghfast.top/https://gh-proxy.com/" + direct
	if !CanAccelerateRuleURL(wrapped) || EffectiveRuleURL(wrapped, true) != "https://gh-proxy.com/"+direct || EffectiveRuleURL(wrapped, false) != direct {
		t.Fatal("proxy normalization lost original URL")
	}
	for _, raw := range []string{"", "https://github.com.attacker.example/rules", "https://user:secret@github.com/a/b", "https://example.org/rules", "https://github.com:8443/a/b"} {
		if CanAccelerateRuleURL(raw) {
			t.Fatal("accepted ineligible URL")
		}
	}
}

func TestChannelDependencyCyclesAndMissingDetoursFailClosed(t *testing.T) {
	nodes, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[
 {"type":"socks","tag":"a","server":"a.example","server_port":1080,"detour":"b"},
 {"type":"socks","tag":"b","server":"b.example","server_port":1080,"detour":"a"},
 {"type":"socks","tag":"c","server":"c.example","server_port":1080,"detour":"a"},
 {"type":"socks","tag":"d","server":"d.example","server_port":1080,"detour":"missing"},
 {"type":"socks","tag":"unresolved-detour","server":"safe.example","server_port":1080}]}`), "s")
	if err != nil {
		t.Fatal(err)
	}
	p := &ChannelPolicy{Selection: NodeSelection{NewNodePolicy: "include"}, Organizer: NodeOrganizer{Sort: "none", Incompatible: "skip"}, DefaultExit: RouteExit{Kind: "direct"}}
	result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil || result.NodeCount != 1 || len(result.Diagnostics) != 4 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !bytes.Contains(result.Content, []byte("safe.example")) || bytes.Contains(result.Content, []byte("d.example")) {
		t.Fatal(string(result.Content))
	}
}

func TestChannelNamesAndDedupKeepSourceDependencies(t *testing.T) {
	var nodes []Node
	for _, source := range []string{"first", "second"} {
		part, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"socks","tag":"entry","server":"same.example","server_port":1080,"detour":"hop"},{"type":"socks","tag":"hop","server":"`+source+`.example","server_port":1080}]}`), source)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, part...)
	}
	nodes[0].OriginTag = "Real name"
	p := &ChannelPolicy{Selection: NodeSelection{NewNodePolicy: "include"}, Organizer: NodeOrganizer{Sort: "none", Incompatible: "error", Deduplicate: true}, DefaultExit: RouteExit{Kind: "direct"}}
	result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil || result.NodeCount != 4 {
		t.Fatalf("%+v %v", result, err)
	}
	root, _ := DecodeDocumentObject(result.Content)
	out := root["outbounds"].([]any)
	first, second := out[0].(map[string]any), out[2].(map[string]any)
	if first["tag"] != "Real name" || first["detour"] == second["detour"] {
		t.Fatal(string(result.Content))
	}
}
