// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// testChannelNodes reads content as a sing-box source and returns its nodes
// as the pool holds them, named by their names in the source.
func testChannelNodes(t *testing.T, sourceID, content string) []ChannelNode {
	t.Helper()
	reading, err := ReadSource([]byte(content), SourceFormatSingBox, SourceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	nodes := []ChannelNode{}
	for _, item := range reading.Kept() {
		outbound, err := json.Marshal(item.Outbound)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, ChannelNode{
			ID: poolNodeID(originSource, sourceID, item.OriginKey), Name: item.Name, Protocol: item.Protocol,
			SourceID: sourceID, OriginName: item.Name, Outbound: outbound,
		})
	}
	return nodes
}

func nodeMembers(ids ...string) []GroupMember {
	members := make([]GroupMember, len(ids))
	for index, id := range ids {
		members[index] = GroupMember{Kind: memberNode, ID: id}
	}
	return members
}

func groupMembers(ids ...string) []GroupMember {
	members := make([]GroupMember, len(ids))
	for index, id := range ids {
		members[index] = GroupMember{Kind: memberGroup, ID: id}
	}
	return members
}

// publishAll is a channel whose one group holds every node.
func publishAll(nodes []ChannelNode, incompatible string) ChannelConfig {
	ids := make([]string, len(nodes))
	for index, node := range nodes {
		ids[index] = node.ID
	}
	return ChannelConfig{IncompatibleNodes: incompatible, DefaultExit: RouteExit{Kind: "direct"}, Routes: []ChannelRoute{}, Groups: []RuleGroup{
		{ID: "all", Name: "All", Enabled: true, Type: "select", Members: nodeMembers(ids...)},
	}}
}

func policyFixture(t *testing.T, format RenderFormat) ([]ChannelNode, *ChannelConfig) {
	t.Helper()
	nodes := testChannelNodes(t, "source-one", `{"outbounds":[{"type":"socks","tag":"Tokyo","server":"tokyo.example.com","server_port":1080},{"type":"socks","tag":"Hong Kong","server":"hk.example.com","server_port":1080}]}`)
	a, b := nodes[0].ID, nodes[1].ID
	p := &ChannelConfig{IncompatibleNodes: "error", DefaultExit: RouteExit{Kind: "group", ID: "main"},
		Groups: []RuleGroup{{Type: "select", ID: "main", Name: "Selected", Enabled: true, Members: nodeMembers(a, b)}},
		Routes: []ChannelRoute{
			{ID: "domain", Enabled: true, Rule: &SubscriptionRule{Kind: "domain_suffix", Value: "example.org"}, Exit: RouteExit{Kind: "group", ID: "main"}},
			{ID: "ipv6", Enabled: true, Rule: &SubscriptionRule{Kind: "ip_cidr", Value: "2001:db8::/32"}, Exit: RouteExit{Kind: "direct"}},
			{ID: "disabled", Enabled: false, Rule: &SubscriptionRule{Kind: "domain", Value: "disabled.example"}, Exit: RouteExit{Kind: "reject"}},
			{ID: "remote", Enabled: true, Exit: RouteExit{Kind: "node", ID: b}, Rule: &SubscriptionRule{Kind: "remote", Remote: &RemoteRuleSet{Name: "Remote rules", URL: "https://raw.githubusercontent.com/example/rules/main/proxy.rules", Format: "source", Accelerated: true}}},
		}}
	if format == RenderFormatMihomo {
		p.Routes[3].Rule.Remote.Format = "mrs"
		p.Routes[3].Rule.Remote.Behavior = "domain"
	}
	if format == RenderFormatLoon {
		p.Routes[3].Rule.Remote.Format = "loon"
	}
	return nodes, p
}

func TestChannelYAMLUsesBlockCollectionsWithoutChangingValues(t *testing.T) {
	for _, content := range []string{
		"{}",
		`{custom-counter: 9007199254740993, custom-string: 'true', dns: {enable: true, nameserver: [1.1.1.1, 8.8.8.8]}}`,
		"# retained comment\ncustom-text: |\n  first line\n  second line\ncustom-string: '00123'\ndns: {enable: true}\n",
		`# "\U0001F680" is literal comment text 🚀
说明: "🏠 \"quoted\" \\U0001F511\nnext line\tend"
escaped: "\\U0001F3E0"
single-quoted: '\U0001F3E0'
plain: \U0001F3E0
custom-text: |
  "\U0001F3E0" is literal block text
unicode-block: |
  🏠 first line
  🐟 second line
tagged: !!str "🔑 加密货币"
custom-tag: !label "🚀 全球服务"
"🏠 key": "🐟 value" # "\U0001F41F" is literal
supplementary: "𠮷 👩🏽‍💻 🇯🇵"
`,
	} {
		nodes, policy := policyFixture(t, RenderFormatMihomo)
		policy.Template = &NativeTemplate{Format: RenderFormatMihomo, Content: content}
		result, err := RenderChannelNodes(nodes, RenderFormatMihomo, *policy, nil)
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
		for _, preserved := range []string{"9007199254740993", "'true'", "'00123'", "# retained comment", "custom-text: |", `# "\U0001F680" is literal comment text 🚀`, `# "\U0001F41F" is literal`} {
			if strings.Contains(content, preserved) && !bytes.Contains(result.Content, []byte(preserved)) {
				t.Fatalf("scalar/comment lost: %s in %s", preserved, result.Content)
			}
		}
	}
}

func TestChannelYAMLEmitsUnicodeLiterally(t *testing.T) {
	nodes, policy := policyFixture(t, RenderFormatMihomo)
	nodes[0].Name = "🇯🇵 Tokyo 👩🏽‍💻"
	policy.Groups[0].Name = "🏠 国内服务"
	policy.Routes[3].Rule.Remote.Name = "🚀 全球服务"
	policy.Groups = append(policy.Groups,
		RuleGroup{ID: "crypto", Name: "🔑 加密货币", Type: "select", Enabled: true, Members: groupMembers("main")},
		RuleGroup{ID: "final", Name: "🐟 漏网之鱼", Type: "select", Enabled: true, Members: groupMembers("main")},
		RuleGroup{ID: "selector", Name: "✨ 节点选择", Type: "select", Enabled: true, Members: groupMembers("main")},
	)
	result, err := RenderChannelNodes(nodes, RenderFormatMihomo, *policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{nodes[0].Name, "🏠 国内服务", "🚀 全球服务", "🔑 加密货币", "🐟 漏网之鱼", "✨ 节点选择"} {
		if !bytes.Contains(result.Content, []byte(name)) {
			t.Errorf("export does not contain literal %q", name)
		}
	}
	var root map[string]any
	if err := yaml.Unmarshal(result.Content, &root); err != nil {
		t.Fatal(err)
	}
	groups := root["proxy-groups"].([]any)
	for i, group := range policy.Groups {
		got := groups[i].(map[string]any)
		if got["name"] != group.Name {
			t.Fatalf("group name changed: %v", got)
		}
		wantCandidate := "🏠 国内服务"
		if i == 0 {
			wantCandidate = nodes[0].Name
		}
		if got["proxies"].([]any)[0] != wantCandidate {
			t.Fatalf("group reference changed: %v", got)
		}
	}
	wantRules := []any{"DOMAIN-SUFFIX,example.org,🏠 国内服务", "IP-CIDR6,2001:db8::/32,DIRECT,no-resolve", "RULE-SET,🚀 全球服务,Hong Kong", "MATCH,🏠 国内服务"}
	if !reflect.DeepEqual(exitRules(RenderFormatMihomo, root["rules"].([]any)), wantRules) || root["rule-providers"].(map[string]any)["🚀 全球服务"] == nil {
		t.Fatalf("rule references changed: %s", result.Content)
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
			result, err := RenderChannelNodes(nodes, format, *p, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.NodeCount != 2 || !bytes.Contains(result.Content, []byte("https://gh-proxy.com/https://raw.githubusercontent.com/")) || !bytes.Contains(result.Content, []byte("9007199254740993")) || bytes.Contains(result.Content, []byte("disabled.example")) {
				t.Fatal(string(result.Content))
			}
			again, err := RenderChannelNodes(nodes, format, *p, nil)
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
				rules := exitRules(format, route["rules"].([]any))
				if len(rules) != 3 || rules[0].(map[string]any)["outbound"] != "Selected" || rules[2].(map[string]any)["outbound"] != nodes[1].Name {
					t.Fatal(rules)
				}
				set := route["rule_set"].([]any)[0].(map[string]any)
				if set["format"] != "source" || set["tag"] != "Remote rules" || set["update_interval"] != nil || !reflect.DeepEqual(set["http_client"], map[string]any{"detour": "direct"}) {
					t.Fatal(set)
				}
			} else {
				if err := yaml.Unmarshal(result.Content, &root); err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(result.Content, []byte("# retained comment")) {
					t.Fatal("template comment lost")
				}
				rules := exitRules(format, root["rules"].([]any))
				if len(rules) != 4 || rules[1] != "IP-CIDR6,2001:db8::/32,DIRECT,no-resolve" || rules[3] != "MATCH,Selected" {
					t.Fatal(rules)
				}
				provider := root["rule-providers"].(map[string]any)["Remote rules"].(map[string]any)
				if provider["format"] != "mrs" || provider["behavior"] != "domain" || provider["interval"] != nil || rules[2] != "RULE-SET,Remote rules,Hong Kong" {
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
	result, err := RenderChannelNodes(nodes[1:], RenderFormatSingBox, *p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != 1 || bytes.Contains(result.Content, []byte("tokyo.example.com")) {
		t.Fatal(string(result.Content))
	}
	root, _ := DecodeDocumentObject(result.Content)
	rules := exitRules(RenderFormatSingBox, root["route"].(map[string]any)["rules"].([]any))
	if rules[0].(map[string]any)["action"] != "reject" || rules[len(rules)-1].(map[string]any)["action"] != "reject" {
		t.Fatal(rules)
	}
	if len(p.Groups[0].Members) != 2 {
		t.Fatal("render mutated membership")
	}
	restored, err := RenderChannelNodes(nodes, RenderFormatSingBox, *p, nil)
	if err != nil || restored.NodeCount != 2 {
		t.Fatal(restored, err)
	}
}

func TestChannelSkipsANodeItCannotNameAndRendersTheRest(t *testing.T) {
	nodes := testChannelNodes(t, "src", `{"outbounds":[
		{"type":"shadowsocks","tag":"HK, 01","server":"1.2.3.4","server_port":8388,"method":"aes-128-gcm","password":"secret-pass"},
		{"type":"shadowsocks","tag":"JP 01","server":"1.2.3.5","server_port":8388,"method":"aes-128-gcm","password":"secret-pass"}]}`)
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		result, err := RenderChannelNodes(nodes, format, publishAll(nodes, "skip"), nil)
		if err != nil || result.NodeCount != 1 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != DiagnosticInvalidMetadata ||
			result.PreviewDiagnostics[0].NodeName != "HK, 01" {
			t.Fatalf("%s: %+v, %v", format, result.PreviewDiagnostics, err)
		}
		if _, err := RenderChannelNodes(nodes, format, publishAll(nodes, "error"), nil); err == nil {
			t.Fatalf("%s: a skipped node passed incompatible_nodes=error", format)
		}
	}
}

func TestChannelPublishesOnlyNodesItsGroupsName(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatSingBox)
	more := testChannelNodes(t, "new-source", `{"outbounds":[{"type":"socks","tag":"Amsterdam","server":"ams.example.com","server_port":1080},{"type":"socks","tag":"Berlin","server":"ber.example.com","server_port":1080},{"type":"socks","tag":"Paris","server":"par.example.com","server_port":1080}]}`)
	// Pool order puts the new nodes first; the channel keeps it.
	pool := append(append([]ChannelNode{}, more...), nodes...)
	result, err := RenderChannelNodes(pool, RenderFormatSingBox, *p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != 2 || bytes.Contains(result.Content, []byte("ams.example.com")) || !result.Carries(nodes[0].ID) || result.Carries(more[0].ID) {
		t.Fatalf("an ungrouped node was published: %s", result.Content)
	}
	// An exit rule's exit and the final exit publish their nodes too, in pool
	// order; a disabled exit rule's exit does not.
	p.Routes[1].Exit = RouteExit{Kind: "node", ID: more[1].ID}
	p.Routes[2].Exit = RouteExit{Kind: "node", ID: more[2].ID}
	p.DefaultExit = RouteExit{Kind: "node", ID: more[0].ID}
	result, err = RenderChannelNodes(pool, RenderFormatSingBox, *p, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := DecodeDocumentObject(result.Content)
	var tags []any
	for _, value := range root["outbounds"].([]any) {
		tags = append(tags, value.(map[string]any)["tag"])
	}
	if !slices.Equal(tags[:4], []any{"Amsterdam", "Berlin", "Tokyo", "Hong Kong"}) || result.NodeCount != 4 || bytes.Contains(result.Content, []byte("par.example.com")) {
		t.Fatalf("published nodes = %v", tags)
	}
	if root["route"].(map[string]any)["final"] != "Amsterdam" {
		t.Fatal(root["route"])
	}
}

func TestInvalidChannelPoliciesExposePathsWithoutInput(t *testing.T) {
	for name, change := range map[string]func(*ChannelConfig){
		"native format": func(p *ChannelConfig) { p.Routes[3].Rule.Remote.Format = "mrs" },
		"invalid node":  func(p *ChannelConfig) { p.Routes[0].Exit = RouteExit{Kind: "node", ID: "not valid"} },
		"template conflict": func(p *ChannelConfig) {
			p.Template = &NativeTemplate{Format: RenderFormatSingBox, Content: `{"route":{"rules":[{"password":"sensitive-content"}]}}`}
		},
		"template duplicate key": func(p *ChannelConfig) {
			p.Template = &NativeTemplate{Format: RenderFormatSingBox, Content: `{"log":{},"log":{}}`}
		},
		"disabled fallback": func(p *ChannelConfig) { p.Groups[0].Enabled = false },
		"invalid CIDR":      func(p *ChannelConfig) { p.Routes[1].Rule.Value = "not-an-ip" },
		"URL credentials":   func(p *ChannelConfig) { p.Routes[3].Rule.Remote.URL = "https://secret:credential@example.com/path" },
	} {
		t.Run(name, func(t *testing.T) {
			_, p := policyFixture(t, RenderFormatSingBox)
			change(p)
			err := ValidateChannelConfig(*p, RenderFormatSingBox)
			var e *PolicyError
			if !errors.As(err, &e) || e.Path == "" || strings.Contains(err.Error(), "sensitive-content") || strings.Contains(err.Error(), "credential@example") {
				t.Fatal(err)
			}
		})
	}
	_, p := policyFixture(t, RenderFormatMihomo)
	p.Routes[3].Rule.Remote.Behavior = "classical"
	if err := ValidateChannelConfig(*p, RenderFormatMihomo); err == nil {
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
	nodes := testChannelNodes(t, "s", `{"outbounds":[
 {"type":"socks","tag":"a","server":"a.example","server_port":1080,"detour":"b"},
 {"type":"socks","tag":"b","server":"b.example","server_port":1080,"detour":"a"},
 {"type":"socks","tag":"c","server":"c.example","server_port":1080,"detour":"a"},
 {"type":"socks","tag":"d","server":"d.example","server_port":1080,"detour":"missing"},
 {"type":"socks","tag":"unresolved-detour","server":"safe.example","server_port":1080}]}`)
	result, err := RenderChannelNodes(nodes, RenderFormatSingBox, publishAll(nodes, "skip"), nil)
	if err != nil || result.NodeCount != 1 || len(result.Diagnostics) != 4 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !bytes.Contains(result.Content, []byte("safe.example")) || bytes.Contains(result.Content, []byte("d.example")) {
		t.Fatal(string(result.Content))
	}
}

func TestChannelNamesKeepDistinctSourceDependencies(t *testing.T) {
	var nodes []ChannelNode
	for _, source := range []string{"first", "second"} {
		nodes = append(nodes, testChannelNodes(t, source, `{"outbounds":[{"type":"socks","tag":"entry","server":"same.example","server_port":1080,"detour":"hop"},{"type":"socks","tag":"hop","server":"`+source+`.example","server_port":1080}]}`)...)
	}
	// Display names are unique in the pool; a detour still names its source's
	// node by its name there.
	nodes[0].Name, nodes[1].Name, nodes[2].Name, nodes[3].Name = "Real name", "hop", "entry 2", "hop 2"
	result, err := RenderChannelNodes(nodes, RenderFormatSingBox, publishAll(nodes, "error"), nil)
	if err != nil || result.NodeCount != 4 {
		t.Fatalf("%+v %v", result, err)
	}
	root, _ := DecodeDocumentObject(result.Content)
	out := root["outbounds"].([]any)
	first, second := out[0].(map[string]any), out[2].(map[string]any)
	if first["tag"] != "Real name" || first["detour"] != "hop" || second["detour"] != "hop 2" {
		t.Fatal(string(result.Content))
	}
	// A detour to a node the channel does not publish is unresolved.
	config := publishAll(nodes[:1], "skip")
	result, err = RenderChannelNodes(nodes, RenderFormatSingBox, config, nil)
	if err != nil || result.NodeCount != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != DiagnosticUnresolvedDependency {
		t.Fatalf("%+v %v", result, err)
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
			for _, kind := range test.builtins {
				group.Members = append(group.Members, GroupMember{Kind: memberBuiltin, ID: kind})
			}
			group.HealthCheck = &GroupHealthCheck{URL: "https://probe.example/check", Interval: 3600, Tolerance: 0}
			before, _ := json.Marshal(policy)
			result, err := RenderChannelNodes(nodes, test.format, *policy, nil)
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
		"missing type":      func(g *RuleGroup) { g.Type = "" },
		"unknown type":      func(g *RuleGroup) { g.Type = "invalid" },
		"sing-box fallback": func(g *RuleGroup) { g.Type = "fallback" },
		"sing-box reject":   func(g *RuleGroup) { g.Members = append(g.Members, GroupMember{Kind: memberBuiltin, ID: "reject"}) },
		"unknown builtin":   func(g *RuleGroup) { g.Members = append(g.Members, GroupMember{Kind: memberBuiltin, ID: "unknown"}) },
		"duplicate builtin": func(g *RuleGroup) {
			g.Members = append(g.Members, GroupMember{Kind: memberBuiltin, ID: "direct"}, GroupMember{Kind: memberBuiltin, ID: "direct"})
		},
		"unknown kind":       func(g *RuleGroup) { g.Members = append(g.Members, GroupMember{Kind: "other", ID: "x"}) },
		"duplicate node":     func(g *RuleGroup) { g.Members = append(g.Members, g.Members[0]) },
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
			if err := ValidateChannelConfig(*policy, RenderFormatSingBox); !errors.As(err, &problem) || !strings.HasPrefix(problem.Path, "groups[0].") || strings.Contains(err.Error(), "secret") {
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
			policy.Routes = []ChannelRoute{}
			group := &policy.Groups[0]
			group.Members = []GroupMember{{Kind: memberBuiltin, ID: kind}}
			result, err := RenderChannelNodes(nil, format, *policy, nil)
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
			if kind != "select" {
				result, err := RenderChannelNodes(nodes[1:], format, *policy, nil)
				if err != nil || !bytes.Contains(result.Content, []byte("Selected")) {
					t.Fatal("remaining automatic candidate lost", err)
				}
				if format == RenderFormatSingBox {
					root, _ := DecodeDocumentObject(result.Content)
					for _, rule := range exitRules(format, root["route"].(map[string]any)["rules"].([]any)) {
						if rule.(map[string]any)["action"] == "reject" {
							t.Fatal("automatic group followed unavailable fixed default")
						}
					}
				}
			}
			result, err := RenderChannelNodes(nil, format, *policy, nil)
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
			group.Members = []GroupMember{{Kind: memberNode, ID: nodes[1].ID}, {Kind: memberBuiltin, ID: "direct"}, {Kind: memberNode, ID: nodes[0].ID}}
			result, err := RenderChannelNodes(nodes, format, *policy, nil)
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
		})
	}
}

func TestManualGroupFirstCardIsInitialDefault(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, policy := policyFixture(t, format)
			group := &policy.Groups[0]
			group.Members = []GroupMember{{Kind: memberBuiltin, ID: "direct"}, {Kind: memberNode, ID: nodes[1].ID}, {Kind: memberNode, ID: nodes[0].ID}}
			rendered, err := RenderChannelNodes(nodes, format, *policy, nil)
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

// renderedRules summarizes a rendered channel's rules as "KIND,value,target"
// lines, and its remote rule sets as "tag=url".
func renderedRules(t *testing.T, format RenderFormat, content []byte) ([]string, []string) {
	t.Helper()
	var rules, remotes []string
	switch format {
	case RenderFormatSingBox:
		root, err := DecodeDocumentObject(content)
		if err != nil {
			t.Fatal(err)
		}
		route := root["route"].(map[string]any)
		for _, value := range route["rules"].([]any) {
			rule := value.(map[string]any)
			target, _ := rule["outbound"].(string)
			if rule["action"] == "reject" {
				target = "REJECT"
			}
			for _, kind := range []string{"domain", "domain_suffix", "domain_keyword", "ip_cidr", "rule_set"} {
				values, _ := rule[kind].([]any)
				for _, value := range values {
					rules = append(rules, kind+","+value.(string)+","+target)
				}
			}
		}
		for _, value := range route["rule_set"].([]any) {
			set := value.(map[string]any)
			remotes = append(remotes, set["tag"].(string)+"="+set["url"].(string))
		}
	case RenderFormatMihomo:
		var root map[string]any
		if err := yaml.Unmarshal(content, &root); err != nil {
			t.Fatal(err)
		}
		for _, value := range exitRules(format, root["rules"].([]any)) {
			rules = append(rules, value.(string))
		}
		for name, value := range root["rule-providers"].(map[string]any) {
			remotes = append(remotes, name+"="+value.(map[string]any)["url"].(string))
		}
		slices.Sort(remotes)
	default:
		sections := strings.SplitN(string(content), "[Remote Rule]\n", 2)
		lines := strings.Split(sections[1], "\n")
		for index, line := range lines {
			if line == "" {
				rules = lines[index+2:]
				break
			}
			remotes = append(remotes, line)
		}
		rules = slices.DeleteFunc(rules, func(line string) bool { return line == "" })
	}
	return rules, remotes
}

func TestChannelRoutesEmitRuleSetsInOrderWithTheirExits(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, p := policyFixture(t, format)
			remote := func(url string) *SubscriptionRule {
				value := &RemoteRuleSet{Name: "Shared", URL: url, Format: map[RenderFormat]string{RenderFormatSingBox: "source", RenderFormatMihomo: "yaml", RenderFormatLoon: "loon"}[format]}
				if format == RenderFormatMihomo {
					value.Behavior = "domain"
				}
				return &SubscriptionRule{Kind: "remote", Remote: value}
			}
			shared, other := "https://rules.example/shared.list", "https://rules.example/other.list"
			ruleSets := map[string]ChannelRuleSet{
				"streaming": {Client: format, Rules: []SubscriptionRule{
					{Kind: "domain", Value: "a.example"}, {Kind: "domain_suffix", Value: "b.example"}, *remote(shared), {Kind: "ip_cidr", Value: "10.0.0.0/8"},
				}},
				// The same remote in another set merges with the first, and it and
				// the repeated domain, which the first rules already take, go.
				"other": {Client: format, Rules: []SubscriptionRule{*remote(shared), {Kind: "domain", Value: "a.example"}}},
			}
			p.Routes = []ChannelRoute{
				{ID: "first", Enabled: true, Rule: &SubscriptionRule{Kind: "domain", Value: "first.example"}, Exit: RouteExit{Kind: "direct"}},
				{ID: "streaming", Enabled: true, RuleSetID: "streaming", Exit: RouteExit{Kind: "group", ID: "main"}},
				{ID: "disabled", Enabled: false, RuleSetID: "other", Exit: RouteExit{Kind: "direct"}},
				{ID: "other", Enabled: true, RuleSetID: "other", Exit: RouteExit{Kind: "reject"}},
				// Another remote under the same name is renamed.
				{ID: "own", Enabled: true, Rule: remote(other), Exit: RouteExit{Kind: "direct"}},
			}
			result, err := RenderChannelNodes(nodes, format, *p, ruleSets)
			if err != nil {
				t.Fatal(err)
			}
			rules, remotes := renderedRules(t, format, result.Content)
			want := map[RenderFormat][]string{
				RenderFormatSingBox: {"domain,first.example,direct", "domain,a.example,Selected", "domain_suffix,b.example,Selected", "rule_set,Shared,Selected",
					"ip_cidr,10.0.0.0/8,Selected", "rule_set,Shared-2,direct"},
				RenderFormatMihomo: {"DOMAIN,first.example,DIRECT", "DOMAIN,a.example,Selected", "DOMAIN-SUFFIX,b.example,Selected", "RULE-SET,Shared,Selected",
					"IP-CIDR,10.0.0.0/8,Selected,no-resolve", "RULE-SET,Shared-2,DIRECT", "MATCH,Selected"},
				RenderFormatLoon: {"DOMAIN,first.example,DIRECT", "DOMAIN,a.example,Selected", "DOMAIN-SUFFIX,b.example,Selected",
					"IP-CIDR,10.0.0.0/8,Selected,no-resolve", "FINAL,Selected"},
			}[format]
			wantRemotes := []string{"Shared-2=" + other, "Shared=" + shared}
			if format == RenderFormatSingBox {
				wantRemotes = []string{"Shared=" + shared, "Shared-2=" + other}
			}
			if format == RenderFormatLoon {
				wantRemotes = []string{shared + ",policy=Selected,tag=Shared,enabled=true", other + ",policy=DIRECT,tag=Shared-2,enabled=true"}
			}
			if !slices.Equal(rules, want) || !slices.Equal(remotes, wantRemotes) {
				t.Fatalf("rules %q\nremotes %q", rules, remotes)
			}
			if format == RenderFormatSingBox {
				// The two matches in a row with one exit are one rule; a rule set
				// stays apart.
				root, _ := DecodeDocumentObject(result.Content)
				if exit := exitRules(format, root["route"].(map[string]any)["rules"].([]any)); len(exit) != 5 {
					t.Fatalf("sing-box exit rules = %v", exit)
				}
			}
		})
	}
}

func TestChannelRoutesNeedRuleSetsOfTheirClient(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatMihomo)
	p.Routes = append(p.Routes, ChannelRoute{ID: "set", Enabled: false, RuleSetID: "set", Exit: RouteExit{Kind: "direct"}})
	for code, ruleSets := range map[string]map[string]ChannelRuleSet{
		"rule_set_not_found":       {},
		"rule_set_client_mismatch": {"set": {Client: RenderFormatSingBox, Rules: []SubscriptionRule{}}},
	} {
		var problem *PolicyError
		if _, err := RenderChannelNodes(nodes, RenderFormatMihomo, *p, ruleSets); !errors.As(err, &problem) || problem.Path != "routes[4].rule_set_id" || problem.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}
}

func TestGroupMatchTakesPoolNodesAfterMembers(t *testing.T) {
	one := testChannelNodes(t, "source-one", `{"outbounds":[{"type":"socks","tag":"Tokyo","server":"tokyo.example.com","server_port":1080},{"type":"socks","tag":"Hong Kong","server":"hk.example.com","server_port":1080}]}`)
	two := testChannelNodes(t, "source-two", `{"outbounds":[{"type":"socks","tag":"Osaka","server":"osaka.example.com","server_port":1080},{"type":"http","tag":"Kyoto","server":"kyoto.example.com","server_port":8080}]}`)
	fleet := one[0]
	fleet.ID, fleet.Name, fleet.SourceID, fleet.OriginName = "fleet-node", "Fleet Tokyo", "", ""
	pool := append(append([]ChannelNode{fleet}, one...), two...)
	group := RuleGroup{ID: "main", Name: "Main", Enabled: true, Type: "select", Members: nodeMembers(one[1].ID)}
	config := ChannelConfig{IncompatibleNodes: "error", DefaultExit: RouteExit{Kind: "group", ID: "main"}, Groups: []RuleGroup{group}, Routes: []ChannelRoute{}}
	for _, test := range []struct {
		match GroupMatch
		want  []any
	}{
		{GroupMatch{Include: []string{"o"}, Exclude: []string{"/^Kyo/"}, Origins: []string{"fleet", "source:source-one"}, Protocols: []string{"socks"}}, []any{"Hong Kong", "Fleet Tokyo", "Tokyo"}},
		{GroupMatch{Include: []string{}, Exclude: []string{}, Origins: []string{"source:source-two"}, Protocols: []string{"http"}}, []any{"Hong Kong", "Kyoto"}},
		{GroupMatch{Include: []string{}, Exclude: []string{"tokyo"}, Origins: []string{}, Protocols: []string{}}, []any{"Hong Kong", "Osaka", "Kyoto"}},
		{GroupMatch{Include: []string{}, Exclude: []string{}, Origins: []string{}, Protocols: []string{}}, []any{"Hong Kong"}},
	} {
		config.Groups[0].Match = &test.match
		result, err := RenderChannelNodes(pool, RenderFormatMihomo, config, nil)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]any
		if err := yaml.Unmarshal(result.Content, &root); err != nil {
			t.Fatal(err)
		}
		candidates := root["proxy-groups"].([]any)[0].(map[string]any)["proxies"].([]any)
		if !slices.Equal(candidates, test.want) || result.NodeCount != len(test.want) {
			t.Fatalf("match %+v took %v, published %d", test.match, candidates, result.NodeCount)
		}
	}
	if len(config.Groups[0].Members) != 1 {
		t.Fatal("render changed the group's members")
	}
	config.Groups[0].Match = &GroupMatch{Include: []string{"/(/"}, Exclude: []string{}, Origins: []string{"source:"}, Protocols: []string{}}
	var problem *PolicyError
	if err := ValidateChannelConfig(config, RenderFormatMihomo); !errors.As(err, &problem) || problem.Path != "groups[0].match.include[0]" || problem.Code != "invalid_regular_expression" {
		t.Fatal(err)
	}
	config.Groups[0].Match.Include = []string{}
	if err := ValidateChannelConfig(config, RenderFormatMihomo); !errors.As(err, &problem) || problem.Path != "groups[0].match.origins[0]" {
		t.Fatal(err)
	}
}

func TestChannelGroupReferencesRenderInDependencyOrder(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, p := policyFixture(t, format)
			leaf := p.Groups[0]
			leaf.ID, leaf.Name = "leaf", "Leaf"
			middle := RuleGroup{ID: "middle", Name: "Middle", Enabled: true, Type: "select", Members: groupMembers("leaf")}
			parent := RuleGroup{ID: "main", Name: "Parent", Enabled: true, Type: "select", Members: []GroupMember{
				{Kind: memberGroup, ID: "middle"}, {Kind: memberBuiltin, ID: "direct"}, {Kind: memberGroup, ID: "leaf"}, {Kind: memberNode, ID: nodes[1].ID},
			}}
			p.Groups = []RuleGroup{parent, middle, leaf}
			result, err := RenderChannelNodes(nodes, format, *p, nil)
			if err != nil {
				t.Fatal(err)
			}
			if format == RenderFormatLoon {
				content := string(result.Content)
				if strings.Index(content, "Leaf =") > strings.Index(content, "Middle =") || strings.Index(content, "Middle =") > strings.Index(content, "Parent =") || !strings.Contains(content, "Parent = select,Middle,DIRECT,Leaf,Hong Kong") {
					t.Fatal(content)
				}
				return
			}
			var root map[string]any
			if err := yaml.Unmarshal(result.Content, &root); err != nil {
				t.Fatal(err)
			}
			key, names, candidates, direct := "proxy-groups", "name", "proxies", "DIRECT"
			if format == RenderFormatSingBox {
				key, names, candidates, direct = "outbounds", "tag", "outbounds", "direct"
			}
			groups := root[key].([]any)
			groups = groups[len(groups)-3:]
			if groups[0].(map[string]any)[names] != "Leaf" || groups[1].(map[string]any)[names] != "Middle" {
				t.Fatal(groups)
			}
			got := groups[2].(map[string]any)
			if got[names] != "Parent" || !reflect.DeepEqual(got[candidates], []any{"Middle", direct, "Leaf", "Hong Kong"}) {
				t.Fatal(got)
			}
		})
	}
}

func TestChannelUnavailableNestedGroupDoesNotChangeManualExit(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		for _, kind := range []string{"select", "url-test"} {
			t.Run(string(format)+"/"+kind, func(t *testing.T) {
				nodes, p := policyFixture(t, format)
				leaf := p.Groups[0]
				leaf.ID, leaf.Name = "leaf", "Leaf"
				parent := RuleGroup{ID: "main", Name: "Parent", Enabled: true, Type: kind, Members: []GroupMember{{Kind: memberGroup, ID: "leaf"}, {Kind: memberNode, ID: nodes[1].ID}}}
				p.Groups = []RuleGroup{parent, leaf}
				p.Routes = []ChannelRoute{{ID: "rule", Enabled: true, Rule: &SubscriptionRule{Kind: "domain", Value: "example.org"}, Exit: RouteExit{Kind: "group", ID: "main"}}}
				result, err := RenderChannelNodes(nodes[1:], format, *p, nil)
				if err != nil {
					t.Fatal(err)
				}
				if format == RenderFormatLoon {
					target := "Parent"
					if kind == "select" {
						target = "REJECT"
					}
					if !strings.Contains(string(result.Content), "DOMAIN,example.org,"+target) || !strings.Contains(string(result.Content), "Parent = "+kind+",Hong Kong") {
						t.Fatal(string(result.Content))
					}
					return
				}
				var root map[string]any
				if err := yaml.Unmarshal(result.Content, &root); err != nil {
					t.Fatal(err)
				}
				if format == RenderFormatSingBox {
					rule := exitRules(format, root["route"].(map[string]any)["rules"].([]any))[0].(map[string]any)
					if (kind == "select" && rule["action"] != "reject") || (kind == "url-test" && rule["outbound"] != "Parent") {
						t.Fatal(rule)
					}
				} else {
					target := "Parent"
					if kind == "select" {
						target = "REJECT"
					}
					if exitRules(format, root["rules"].([]any))[0] != "DOMAIN,example.org,"+target {
						t.Fatal(root)
					}
				}
			})
		}
	}
}

func TestChannelGroupReferenceValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ChannelConfig)
		code   string
	}{
		{"missing", func(p *ChannelConfig) { p.Groups[0].Members = groupMembers("missing") }, "group_not_found"},
		{"disabled", func(p *ChannelConfig) { p.Groups[0].Members = groupMembers("child"); p.Groups[1].Enabled = false }, "group_not_enabled"},
		{"self", func(p *ChannelConfig) { p.Groups[0].Members = groupMembers("main") }, "cyclic_group_reference"},
		{"duplicate", func(p *ChannelConfig) { p.Groups[0].Members = groupMembers("child", "child") }, "duplicate_member"},
		{"cycle", func(p *ChannelConfig) {
			p.Groups[0].Members = groupMembers("child")
			p.Groups[1].Members = groupMembers("main")
		}, "cyclic_group_reference"},
		{"indirect cycle", func(p *ChannelConfig) {
			p.Groups[0].Members = groupMembers("child")
			p.Groups[1].Members = groupMembers("last")
			p.Groups = append(p.Groups, RuleGroup{ID: "last", Name: "Last", Enabled: true, Type: "select", Members: groupMembers("main")})
		}, "cyclic_group_reference"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, p := policyFixture(t, RenderFormatMihomo)
			p.Groups = append(p.Groups, RuleGroup{ID: "child", Name: "Child", Type: "select", Enabled: true, Members: []GroupMember{}})
			test.change(p)
			var problem *PolicyError
			if err := ValidateChannelConfig(*p, RenderFormatMihomo); !errors.As(err, &problem) || problem.Code != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

func TestChannelOwnRemoteNamesAreUniqueIncludingDisabledRoutes(t *testing.T) {
	_, p := policyFixture(t, RenderFormatMihomo)
	route := p.Routes[3]
	route.ID, route.Enabled = "other", false
	p.Routes = append(p.Routes, route)
	var problem *PolicyError
	if err := ValidateChannelConfig(*p, RenderFormatMihomo); !errors.As(err, &problem) || problem.Path != "routes[4].rule.remote.name" || problem.Code != "duplicate_name" {
		t.Fatal(err)
	}
	remote := *route.Rule.Remote
	remote.Name = "remote rules"
	p.Routes[4].Rule = &SubscriptionRule{Kind: "remote", Remote: &remote}
	if err := ValidateChannelConfig(*p, RenderFormatMihomo); err != nil {
		t.Fatal(err)
	}
}

func TestChannelFollowsPoolOrderWithoutChangingNamesOrReferences(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, policy := policyFixture(t, format)
			other := testChannelNodes(t, "other", `{"type":"socks","tag":"Tokyo","server":"other.example.com","server_port":1080}`)
			other[0].Name = "Tokyo 2"
			nodes = append(nodes, other...)
			policy.Groups[0].Members = append(policy.Groups[0].Members, GroupMember{Kind: memberNode, ID: other[0].ID})
			before, err := RenderChannelNodes(nodes, format, *policy, nil)
			if err != nil {
				t.Fatal(err)
			}
			after, err := RenderChannelNodes([]ChannelNode{nodes[2], nodes[1], nodes[0]}, format, *policy, nil)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Index(string(after.Content), "other.example.com") >= strings.Index(string(after.Content), "tokyo.example.com") {
				t.Fatal("pool order ignored")
			}
			if format == RenderFormatLoon {
				first := strings.SplitN(string(before.Content), "[Proxy Group]", 2)
				second := strings.SplitN(string(after.Content), "[Proxy Group]", 2)
				if first[1] != second[1] {
					t.Fatal("group or rule references changed")
				}
				return
			}
			var a, b map[string]any
			if err := yaml.Unmarshal(before.Content, &a); err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(after.Content, &b); err != nil {
				t.Fatal(err)
			}
			key := "proxies"
			if format == RenderFormatSingBox {
				key = "outbounds"
			}
			initial, reordered := a[key].([]any), b[key].([]any)
			reordered[0], reordered[2] = reordered[2], reordered[0]
			if !reflect.DeepEqual(initial, reordered) {
				t.Fatalf("node/group definitions changed: %v != %v", initial, reordered)
			}
			delete(a, key)
			delete(b, key)
			if !reflect.DeepEqual(a, b) {
				t.Fatal("routing changed")
			}
		})
	}
}

// exitRules leaves out the rules every channel of format starts with.
func exitRules(format RenderFormat, rules []any) []any {
	return rules[len(leadingRules(format)):]
}

// TestChannelRulesStartWithLeakGuards puts sing-box's sniffing, DNS hijack,
// STUN reject and private-address bypass, and mihomo's STUN reject, before
// every exit rule, and matches IP rules by the connection's own address.
func TestChannelRulesStartWithLeakGuards(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatSingBox)
	result, err := RenderChannelNodes(nodes, RenderFormatSingBox, *p, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := DecodeDocumentObject(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, value := range root["route"].(map[string]any)["rules"].([]any)[:4] {
		rule := value.(map[string]any)
		actions = append(actions, fmt.Sprintf("%v|%v|%v|%v", rule["action"], rule["protocol"], rule["ip_is_private"], rule["outbound"]))
	}
	if want := []string{"sniff|<nil>|<nil>|<nil>", "hijack-dns|[dns]|<nil>|<nil>", "reject|[stun]|<nil>|<nil>", "route|<nil>|true|direct"}; !slices.Equal(actions, want) {
		t.Fatalf("leading sing-box rules = %q", actions)
	}
	nodes, p = policyFixture(t, RenderFormatMihomo)
	result, err = RenderChannelNodes(nodes, RenderFormatMihomo, *p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var mihomo map[string]any
	if err := yaml.Unmarshal(result.Content, &mihomo); err != nil {
		t.Fatal(err)
	}
	if rules := mihomo["rules"].([]any); rules[0] != "AND,((NETWORK,UDP),(DST-PORT,3478/19302-19309)),REJECT" {
		t.Fatalf("leading mihomo rule = %v", rules[0])
	}
}
