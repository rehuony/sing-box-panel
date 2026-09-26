// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
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
	p := &ChannelPolicy{Selection: NodeSelection{IDs: []string{a, b}, NewNodePolicy: "exclude"}, IncompatibleNodes: "error", DefaultExit: RouteExit{Kind: "group", ID: "main"}, Groups: []RuleGroup{{Type: "select", BuiltinNodes: []string{}, ID: "main", Name: "Selected", Enabled: true, NodeIDs: []string{a, b}, Rules: []ChannelRule{
		{ID: "domain", Enabled: true, Kind: "domain_suffix", Value: "example.org", Exit: RouteExit{Kind: "group-default"}},
		{ID: "ipv6", Enabled: true, Kind: "ip_cidr", Value: "2001:db8::/32", Exit: RouteExit{Kind: "direct"}},
		{ID: "disabled", Enabled: false, Kind: "domain", Value: "disabled.example", Exit: RouteExit{Kind: "reject"}},
		{ID: "remote", Enabled: true, Kind: "remote", Exit: RouteExit{Kind: "node", ID: b}, Remote: &RemoteRuleSet{Name: "Remote rules", URL: "https://raw.githubusercontent.com/example/rules/main/proxy.rules", Format: "source", Accelerated: true, UpdateInterval: 3600}},
	}}}}
	if format == RenderFormatMihomo {
		p.Groups[0].Rules[3].Remote.Format = "mrs"
		p.Groups[0].Rules[3].Remote.Behavior = "domain"
	}
	if format == RenderFormatLoon {
		p.Groups[0].Rules[3].Remote.Format = "loon"
		p.Groups[0].Rules[3].Remote.UpdateInterval = 0
	}
	return nodes, p
}

func TestChannelYAMLUsesBlockCollectionsWithoutChangingValues(t *testing.T) {
	for _, content := range []string{
		"{}",
		`{custom-counter: 9007199254740993, custom-string: 'true', dns: {enable: true, nameserver: [1.1.1.1, 8.8.8.8]}}`,
		"# retained comment\ncustom-text: |\n  first line\n  second line\ncustom-string: '00123'\ndns: {enable: true}\n",
	} {
		nodes, policy := policyFixture(t, RenderFormatMihomo)
		policy.Template = &NativeTemplate{Format: RenderFormatMihomo, Content: content}
		result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatMihomo}, policy)
		if err != nil {
			t.Fatal(err)
		}
		var original, rendered map[string]any
		if err := yaml.Unmarshal([]byte(content), &original); err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(result.Content, &rendered); err != nil {
			t.Fatal(err)
		}
		for key, want := range original {
			if !reflect.DeepEqual(rendered[key], want) {
				t.Fatalf("template value %s changed: %v != %v", key, rendered[key], want)
			}
		}
		if !bytes.Contains(result.Content, []byte("proxies:\n  - ")) || !bytes.Contains(result.Content, []byte("\nproxy-groups:\n")) {
			t.Fatalf("output was not formatted: %s", result.Content)
		}
		for _, preserved := range []string{"9007199254740993", "'true'", "'00123'", "# retained comment", "custom-text: |"} {
			if strings.Contains(content, preserved) && !bytes.Contains(result.Content, []byte(preserved)) {
				t.Fatalf("scalar/comment lost: %s in %s", preserved, result.Content)
			}
		}
	}
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

func TestChannelSelectionOrderAndFixedGroupCandidates(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatSingBox)
	more, _, _ := ParseSource(SourceFormatSingBoxJSON, []byte(`{"type":"socks","tag":"Amsterdam","server":"ams.example.com","server_port":1080}`), "new-source")
	nodes = append(nodes, more...)
	p.Selection.NewNodePolicy = "include"
	result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := DecodeDocumentObject(result.Content)
	out := root["outbounds"].([]any)
	if out[0].(map[string]any)["tag"] != "Tokyo" {
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
	p := &ChannelPolicy{Selection: NodeSelection{NewNodePolicy: "include"}, IncompatibleNodes: "skip", DefaultExit: RouteExit{Kind: "direct"}}
	result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatSingBox}, p)
	if err != nil || result.NodeCount != 1 || len(result.Diagnostics) != 4 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !bytes.Contains(result.Content, []byte("safe.example")) || bytes.Contains(result.Content, []byte("d.example")) {
		t.Fatal(string(result.Content))
	}
}

func TestChannelNamesKeepDistinctSourceDependencies(t *testing.T) {
	var nodes []Node
	for _, source := range []string{"first", "second"} {
		part, _, err := ParseSource(SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"socks","tag":"entry","server":"same.example","server_port":1080,"detour":"hop"},{"type":"socks","tag":"hop","server":"`+source+`.example","server_port":1080}]}`), source)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, part...)
	}
	nodes[0].OriginTag = "Real name"
	p := &ChannelPolicy{Selection: NodeSelection{NewNodePolicy: "include"}, IncompatibleNodes: "error", DefaultExit: RouteExit{Kind: "direct"}}
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

func TestStrategyGroupTypesAndExplicitBuiltinCandidates(t *testing.T) {
	for _, test := range []struct {
		name       string
		format     RenderFormat
		kind       string
		builtins   []string
		nativeKind string
	}{
		{"sing-box select", RenderFormatSingBox, "select", []string{}, "selector"},
		{"sing-box urltest direct", RenderFormatSingBox, "url-test", []string{"direct"}, "urltest"},
		{"mihomo select builtins", RenderFormatMihomo, "select", []string{"direct", "reject"}, "select"},
		{"mihomo urltest", RenderFormatMihomo, "url-test", []string{}, "url-test"},
		{"mihomo fallback", RenderFormatMihomo, "fallback", []string{"direct"}, "fallback"},
	} {
		t.Run(test.name, func(t *testing.T) {
			nodes, policy := policyFixture(t, test.format)
			group := &policy.Groups[0]
			group.Type = test.kind
			group.BuiltinNodes = test.builtins
			group.HealthCheck = &GroupHealthCheck{URL: "https://probe.example/check", Interval: 3600, Tolerance: 0}
			before, _ := json.Marshal(policy)
			result, err := RenderPolicyNodes(nodes, RenderChannel{Format: test.format}, policy)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := yaml.Unmarshal(result.Content, &root); err != nil {
				t.Fatal(err)
			}
			key, candidateKey := "proxy-groups", "proxies"
			if test.format == RenderFormatSingBox {
				key, candidateKey = "outbounds", "outbounds"
			}
			items := root[key].([]any)
			rendered := items[len(items)-1].(map[string]any)
			if rendered["type"] != test.nativeKind {
				t.Fatalf("wrong group type: %v", rendered)
			}
			candidates := rendered[candidateKey].([]any)
			if len(candidates) != 2+len(test.builtins) {
				t.Fatalf("unexpected implicit candidates: %v", candidates)
			}
			if test.kind != "select" {
				if candidates[0] != "Tokyo" || rendered["url"] != "https://probe.example/check" {
					t.Fatal(rendered)
				}
				interval := any(3600)
				if test.format == RenderFormatSingBox {
					interval = "3600s"
					if rendered["idle_timeout"] != interval {
						t.Fatal(rendered)
					}
				}
				if rendered["interval"] != interval {
					t.Fatal(rendered)
				}
				if test.kind == "url-test" && rendered["tolerance"] != 0 {
					t.Fatal(rendered)
				}
				if test.kind == "fallback" && rendered["tolerance"] != nil {
					t.Fatal(rendered)
				}
			} else if rendered["url"] != nil {
				t.Fatal("manual group emitted health check", rendered)
			}
			after, _ := json.Marshal(policy)
			if !bytes.Equal(before, after) {
				t.Fatal("render mutated policy")
			}
		})
	}
}

func TestStrategyGroupCompatibilityAndHealthValidation(t *testing.T) {
	for name, change := range map[string]func(*RuleGroup){
		"missing type":       func(g *RuleGroup) { g.Type = "" },
		"missing builtins":   func(g *RuleGroup) { g.BuiltinNodes = nil },
		"unknown type":       func(g *RuleGroup) { g.Type = "invalid" },
		"sing-box fallback":  func(g *RuleGroup) { g.Type = "fallback" },
		"sing-box reject":    func(g *RuleGroup) { g.BuiltinNodes = []string{"reject"} },
		"unknown builtin":    func(g *RuleGroup) { g.BuiltinNodes = []string{"unknown"} },
		"duplicate builtin":  func(g *RuleGroup) { g.BuiltinNodes = []string{"direct", "direct"} },
		"credential URL":     func(g *RuleGroup) { g.HealthCheck.URL = "https://secret:password@example.com/test" },
		"invalid URL":        func(g *RuleGroup) { g.HealthCheck.URL = "file:///etc/passwd" },
		"short interval":     func(g *RuleGroup) { g.HealthCheck.Interval = 0 },
		"long interval":      func(g *RuleGroup) { g.HealthCheck.Interval = 86401 },
		"negative tolerance": func(g *RuleGroup) { g.HealthCheck.Tolerance = -1 },
		"overflow tolerance": func(g *RuleGroup) { g.HealthCheck.Tolerance = 65536 },
	} {
		t.Run(name, func(t *testing.T) {
			_, policy := policyFixture(t, RenderFormatSingBox)
			policy.Groups[0].HealthCheck = new(policy.Groups[0].healthCheck())
			change(&policy.Groups[0])
			var problem *PolicyError
			if err := ValidateChannelPolicy(policy, RenderFormatSingBox); !errors.As(err, &problem) || !strings.HasPrefix(problem.Path, "policy.groups[0].") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected safe field error, got %v", err)
			}
		})
	}
}

func TestBuiltinOnlyStrategyGroups(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo} {
		for _, kind := range []string{"direct", "reject"} {
			if format == RenderFormatSingBox && kind == "reject" {
				continue
			}
			_, policy := policyFixture(t, format)
			group := &policy.Groups[0]
			group.NodeIDs = []string{}
			group.Rules = []ChannelRule{}
			group.BuiltinNodes = []string{kind}
			result, err := RenderPolicyNodes(nil, RenderChannel{Format: format}, policy)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := yaml.Unmarshal(result.Content, &root); err != nil {
				t.Fatal(err)
			}
			key, candidateKey, target := "proxy-groups", "proxies", strings.ToUpper(kind)
			if format == RenderFormatSingBox {
				key, candidateKey, target = "outbounds", "outbounds", "direct"
			}
			items := root[key].([]any)
			candidates := items[len(items)-1].(map[string]any)[candidateKey].([]any)
			if len(candidates) != 1 || candidates[0] != target || result.NodeCount != 0 {
				t.Fatalf("builtin-only group was not preserved: %s", result.Content)
			}
		}
	}
}

func TestAutomaticGroupsUseRemainingCandidatesAndEmptyGroupsReject(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo} {
		for _, kind := range []string{"select", "url-test", "fallback"} {
			if format == RenderFormatSingBox && kind == "fallback" {
				continue
			}
			nodes, policy := policyFixture(t, format)
			policy.Groups[0].Type = kind
			policy.Groups[0].BuiltinNodes = []string{}
			if kind != "select" {
				result, err := RenderPolicyNodes(nodes[1:], RenderChannel{Format: format}, policy)
				if err != nil || !bytes.Contains(result.Content, []byte("Selected")) {
					t.Fatal("remaining automatic candidate lost", err)
				}
				if format == RenderFormatSingBox && bytes.Contains(result.Content, []byte(`"action": "reject"`)) {
					t.Fatal("automatic group followed unavailable fixed default")
				}
			}
			result, err := RenderPolicyNodes(nil, RenderChannel{Format: format}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(result.Content, []byte("Selected")) {
				t.Fatal("empty group emitted", string(result.Content))
			}
			if format == RenderFormatMihomo && !bytes.Contains(result.Content, []byte("MATCH,REJECT")) {
				t.Fatal(string(result.Content))
			}
			if format == RenderFormatSingBox && !bytes.Contains(result.Content, []byte(`"action": "reject"`)) {
				t.Fatal(string(result.Content))
			}
		}
	}
}

func TestChannelCandidateOrder(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo} {
		t.Run(string(format), func(t *testing.T) {
			nodes, policy := policyFixture(t, format)
			group := &policy.Groups[0]
			group.Type = "url-test"
			group.BuiltinNodes = []string{"direct"}
			group.CandidateOrder = []string{"node:" + group.NodeIDs[1], "builtin:direct", "node:" + group.NodeIDs[0]}
			if err := ValidateChannelPolicy(policy, format); err != nil {
				t.Fatal(err)
			}
			result, err := RenderPolicyNodes(nodes, RenderChannel{Format: format}, policy)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := yaml.Unmarshal(result.Content, &root); err != nil {
				t.Fatal(err)
			}
			key, candidateKey, direct := "proxy-groups", "proxies", "DIRECT"
			if format == RenderFormatSingBox {
				key, candidateKey, direct = "outbounds", "outbounds", "direct"
			}
			items := root[key].([]any)
			actual := items[len(items)-1].(map[string]any)[candidateKey].([]any)
			want := []any{"Hong Kong", direct, "Tokyo"}
			if !slices.Equal(actual, want) {
				t.Fatalf("candidate order = %v, want %v", actual, want)
			}
			for _, order := range [][]string{
				{}, {"node:" + group.NodeIDs[0]},
				{"node:" + group.NodeIDs[0], "node:" + group.NodeIDs[0], "builtin:direct"},
				{"node:" + group.NodeIDs[0], "node:" + group.NodeIDs[1], "builtin:reject"},
				{"node:" + group.NodeIDs[0], "node:unknown", "builtin:direct"},
			} {
				group.CandidateOrder = order
				var problem *PolicyError
				if err := ValidateChannelPolicy(policy, format); !errors.As(err, &problem) || problem.Path != "policy.groups[0].candidate_order" {
					t.Fatalf("invalid candidate order %v: %v", order, err)
				}
			}
		})
	}
}

func TestManualGroupFirstCardIsInitialDefault(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, policy := policyFixture(t, format)
			group := &policy.Groups[0]
			group.BuiltinNodes = []string{"direct"}
			group.CandidateOrder = []string{"builtin:direct", "node:" + group.NodeIDs[1], "node:" + group.NodeIDs[0]}
			rendered, err := RenderPolicyNodes(nodes, RenderChannel{Format: format}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if format == RenderFormatLoon {
				if !strings.Contains(string(rendered.Content), "Selected = select,DIRECT,Hong Kong,Tokyo") {
					t.Fatal(string(rendered.Content))
				}
				return
			}
			var root map[string]any
			if err := yaml.Unmarshal(rendered.Content, &root); err != nil {
				t.Fatal(err)
			}
			groupKey, candidateKey, direct := "proxy-groups", "proxies", "DIRECT"
			if format == RenderFormatSingBox {
				groupKey, candidateKey, direct = "outbounds", "outbounds", "direct"
			}
			groups := root[groupKey].([]any)
			got := groups[len(groups)-1].(map[string]any)
			if !slices.Equal(got[candidateKey].([]any), []any{direct, "Hong Kong", "Tokyo"}) {
				t.Fatal(got)
			}
			if format == RenderFormatSingBox && got["default"] != direct {
				t.Fatal(got)
			}
		})
	}
}

func TestChannelGlobalRuleOrderAllowsDuplicates(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, p := policyFixture(t, format)
			makeRule := func(id, value string, index int, exit string) ChannelRule {
				return ChannelRule{ID: id, Kind: "domain", Value: value, SortIndex: index, Enabled: true, Exit: RouteExit{Kind: exit}}
			}
			first := p.Groups[0]
			first.Rules = []ChannelRule{makeRule("late", "last.example", 90, "direct"), makeRule("same-a", "a.example", 10, "direct")}
			second := first
			second.ID = "second"
			second.Name = "Second"
			second.Rules = []ChannelRule{makeRule("z", "z.example", 10, "direct"), makeRule("same-b", "a.example", 10, "reject"), makeRule("middle", "middle.example", 20, "direct")}
			p.Groups = []RuleGroup{first, second}
			rendered, err := RenderPolicyNodes(nodes, RenderChannel{Format: format}, p)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			if format == RenderFormatSingBox {
				root, _ := DecodeDocumentObject(rendered.Content)
				for _, value := range root["route"].(map[string]any)["rules"].([]any) {
					rule := value.(map[string]any)
					got = append(got, rule["domain"].([]any)[0].(string)+":"+rule["action"].(string))
				}
			} else {
				var rules []any
				if format == RenderFormatMihomo {
					var root map[string]any
					if err := yaml.Unmarshal(rendered.Content, &root); err != nil {
						t.Fatal(err)
					}
					rules = root["rules"].([]any)
				} else {
					for _, line := range strings.Split(string(rendered.Content), "\n") {
						rules = append(rules, line)
					}
				}
				for _, value := range rules {
					line := value.(string)
					if strings.HasPrefix(line, "DOMAIN,") {
						parts := strings.Split(line, ",")
						action := "route"
						if parts[2] == "REJECT" {
							action = "reject"
						}
						got = append(got, parts[1]+":"+action)
					}
				}
			}
			want := []string{"a.example:route", "a.example:reject", "z.example:route", "middle.example:route", "last.example:route"}
			if !slices.Equal(got, want) {
				t.Fatalf("global rule order %v, want %v", got, want)
			}
			if bytes.Contains(rendered.Content, []byte("sort_index")) {
				t.Fatal("management field leaked")
			}
		})
	}
}
