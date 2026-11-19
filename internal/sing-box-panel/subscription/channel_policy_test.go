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
	return ChannelConfig{IncompatibleNodes: incompatible, DefaultExit: RouteExit{Kind: "direct"}, Groups: []RuleGroup{
		{ID: "all", Name: "All", Enabled: true, Type: "select", Members: nodeMembers(ids...), Rules: []ChannelRule{}},
	}}
}

func policyFixture(t *testing.T, format RenderFormat) ([]ChannelNode, *ChannelConfig) {
	t.Helper()
	nodes := testChannelNodes(t, "source-one", `{"outbounds":[{"type":"socks","tag":"Tokyo","server":"tokyo.example.com","server_port":1080},{"type":"socks","tag":"Hong Kong","server":"hk.example.com","server_port":1080}]}`)
	a, b := nodes[0].ID, nodes[1].ID
	p := &ChannelConfig{IncompatibleNodes: "error", DefaultExit: RouteExit{Kind: "group", ID: "main"}, Groups: []RuleGroup{{Type: "select", ID: "main", Name: "Selected", Enabled: true, Members: nodeMembers(a, b), Rules: []ChannelRule{
		{SortIndex: 10, ID: "domain", Enabled: true, Kind: "domain_suffix", Value: "example.org", Exit: RouteExit{Kind: "group-default"}},
		{SortIndex: 20, ID: "ipv6", Enabled: true, Kind: "ip_cidr", Value: "2001:db8::/32", Exit: RouteExit{Kind: "direct"}},
		{ID: "disabled", Enabled: false, Kind: "domain", Value: "disabled.example", Exit: RouteExit{Kind: "reject"}},
		{SortIndex: 30, ID: "remote", Enabled: true, Kind: "remote", Exit: RouteExit{Kind: "node", ID: b}, Remote: &RemoteRuleSet{Name: "Remote rules", URL: "https://raw.githubusercontent.com/example/rules/main/proxy.rules", Format: "source", Accelerated: true}},
	}}}}
	if format == RenderFormatMihomo {
		p.Groups[0].Rules[3].Remote.Format = "mrs"
		p.Groups[0].Rules[3].Remote.Behavior = "domain"
	}
	if format == RenderFormatLoon {
		p.Groups[0].Rules[3].Remote.Format = "loon"
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
		result, err := RenderChannelNodes(nodes, RenderFormatMihomo, *policy)
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
	policy.Groups[0].Rules[3].Remote.Name = "🚀 全球服务"
	policy.Groups = append(policy.Groups,
		RuleGroup{ID: "crypto", Name: "🔑 加密货币", Type: "select", Enabled: true, Members: groupMembers("main"), Rules: []ChannelRule{}},
		RuleGroup{ID: "final", Name: "🐟 漏网之鱼", Type: "select", Enabled: true, Members: groupMembers("main"), Rules: []ChannelRule{}},
		RuleGroup{ID: "selector", Name: "✨ 节点选择", Type: "select", Enabled: true, Members: groupMembers("main"), Rules: []ChannelRule{}},
	)
	result, err := RenderChannelNodes(nodes, RenderFormatMihomo, *policy)
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
	wantRules := []any{"DOMAIN-SUFFIX,example.org,🏠 国内服务", "IP-CIDR6,2001:db8::/32,DIRECT", "RULE-SET,🚀 全球服务,Hong Kong", "MATCH,🏠 国内服务"}
	if !reflect.DeepEqual(root["rules"], wantRules) || root["rule-providers"].(map[string]any)["🚀 全球服务"] == nil {
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
			result, err := RenderChannelNodes(nodes, format, *p)
			if err != nil {
				t.Fatal(err)
			}
			if result.NodeCount != 2 || !bytes.Contains(result.Content, []byte("https://gh-proxy.com/https://raw.githubusercontent.com/")) || !bytes.Contains(result.Content, []byte("9007199254740993")) || bytes.Contains(result.Content, []byte("disabled.example")) {
				t.Fatal(string(result.Content))
			}
			again, err := RenderChannelNodes(nodes, format, *p)
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
				rules := root["rules"].([]any)
				if len(rules) != 4 || rules[1] != "IP-CIDR6,2001:db8::/32,DIRECT" || rules[3] != "MATCH,Selected" {
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
	result, err := RenderChannelNodes(nodes[1:], RenderFormatSingBox, *p)
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
	if len(p.Groups[0].Members) != 2 {
		t.Fatal("render mutated membership")
	}
	restored, err := RenderChannelNodes(nodes, RenderFormatSingBox, *p)
	if err != nil || restored.NodeCount != 2 {
		t.Fatal(restored, err)
	}
}

func TestChannelPublishesOnlyNodesItsGroupsName(t *testing.T) {
	nodes, p := policyFixture(t, RenderFormatSingBox)
	more := testChannelNodes(t, "new-source", `{"outbounds":[{"type":"socks","tag":"Amsterdam","server":"ams.example.com","server_port":1080},{"type":"socks","tag":"Berlin","server":"ber.example.com","server_port":1080},{"type":"socks","tag":"Paris","server":"par.example.com","server_port":1080}]}`)
	// Pool order puts the new nodes first; the channel keeps it.
	pool := append(append([]ChannelNode{}, more...), nodes...)
	result, err := RenderChannelNodes(pool, RenderFormatSingBox, *p)
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != 2 || bytes.Contains(result.Content, []byte("ams.example.com")) || !result.Carries(nodes[0].ID) || result.Carries(more[0].ID) {
		t.Fatalf("an ungrouped node was published: %s", result.Content)
	}
	// A rule exit and the default exit publish their nodes too, in pool order;
	// a disabled rule's exit does not.
	p.Groups[0].Rules[1].Exit = RouteExit{Kind: "node", ID: more[1].ID}
	p.Groups[0].Rules[2].Exit = RouteExit{Kind: "node", ID: more[2].ID}
	p.DefaultExit = RouteExit{Kind: "node", ID: more[0].ID}
	result, err = RenderChannelNodes(pool, RenderFormatSingBox, *p)
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
		"native format": func(p *ChannelConfig) { p.Groups[0].Rules[3].Remote.Format = "mrs" },
		"invalid node":  func(p *ChannelConfig) { p.Groups[0].Rules[0].Exit = RouteExit{Kind: "node", ID: "not valid"} },
		"template conflict": func(p *ChannelConfig) {
			p.Template = &NativeTemplate{Format: RenderFormatSingBox, Content: `{"route":{"rules":[{"password":"sensitive-content"}]}}`}
		},
		"template duplicate key": func(p *ChannelConfig) {
			p.Template = &NativeTemplate{Format: RenderFormatSingBox, Content: `{"log":{},"log":{}}`}
		},
		"disabled fallback": func(p *ChannelConfig) { p.Groups[0].Enabled = false },
		"invalid CIDR":      func(p *ChannelConfig) { p.Groups[0].Rules[1].Value = "not-an-ip" },
		"URL credentials":   func(p *ChannelConfig) { p.Groups[0].Rules[3].Remote.URL = "https://secret:credential@example.com/path" },
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
	p.Groups[0].Rules[3].Remote.Behavior = "classical"
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
	result, err := RenderChannelNodes(nodes, RenderFormatSingBox, publishAll(nodes, "skip"))
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
	result, err := RenderChannelNodes(nodes, RenderFormatSingBox, publishAll(nodes, "error"))
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
	result, err = RenderChannelNodes(nodes, RenderFormatSingBox, config)
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
			result, err := RenderChannelNodes(nodes, test.format, *policy)
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
			group := &policy.Groups[0]
			group.Rules = []ChannelRule{}
			group.Members = []GroupMember{{Kind: memberBuiltin, ID: kind}}
			result, err := RenderChannelNodes(nil, format, *policy)
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
				result, err := RenderChannelNodes(nodes[1:], format, *policy)
				if err != nil || !bytes.Contains(result.Content, []byte("Selected")) {
					t.Fatal("remaining automatic candidate lost", err)
				}
				if format == RenderFormatSingBox && bytes.Contains(result.Content, []byte(`"action": "reject"`)) {
					t.Fatal("automatic group followed unavailable fixed default")
				}
			}
			result, err := RenderChannelNodes(nil, format, *policy)
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
			result, err := RenderChannelNodes(nodes, format, *policy)
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
			rendered, err := RenderChannelNodes(nodes, format, *policy)
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
			makeRule := func(id, value string, index int64, exit string) ChannelRule {
				return ChannelRule{ID: id, Kind: "domain", Value: value, SortIndex: index, Enabled: true, Exit: RouteExit{Kind: exit}}
			}
			first := p.Groups[0]
			first.Rules = []ChannelRule{makeRule("late", "last.example", 90, "direct"), makeRule("same-a", "a.example", -10, "direct"), makeRule("zero", "zero.example", 0, "direct")}
			second := first
			second.ID = "second"
			second.Name = "Second"
			second.Rules = []ChannelRule{makeRule("z", "z.example", -10, "direct"), makeRule("same-b", "a.example", -10, "reject"), makeRule("middle", "middle.example", 20, "direct"), {ID: "omitted", Enabled: true, Kind: "domain", Value: "omitted.example", Exit: RouteExit{Kind: "direct"}}}
			p.Groups = []RuleGroup{first, second}
			rendered, err := RenderChannelNodes(nodes, format, *p)
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
			want := []string{"a.example:route", "a.example:reject", "z.example:route", "omitted.example:route", "zero.example:route", "middle.example:route", "last.example:route"}
			if !slices.Equal(got, want) {
				t.Fatalf("global rule order %v, want %v", got, want)
			}
			if bytes.Contains(rendered.Content, []byte("sort_index")) {
				t.Fatal("management field leaked")
			}
		})
	}
}

func TestChannelGroupReferencesRenderInDependencyOrder(t *testing.T) {
	for _, format := range []RenderFormat{RenderFormatSingBox, RenderFormatMihomo, RenderFormatLoon} {
		t.Run(string(format), func(t *testing.T) {
			nodes, p := policyFixture(t, format)
			leaf := p.Groups[0]
			leaf.ID, leaf.Name, leaf.Rules = "leaf", "Leaf", nil
			leaf.Rules = []ChannelRule{}
			middle := RuleGroup{ID: "middle", Name: "Middle", Enabled: true, Type: "select", Members: groupMembers("leaf"), Rules: []ChannelRule{}}
			parent := RuleGroup{ID: "main", Name: "Parent", Enabled: true, Type: "select", Rules: []ChannelRule{}, Members: []GroupMember{
				{Kind: memberGroup, ID: "middle"}, {Kind: memberBuiltin, ID: "direct"}, {Kind: memberGroup, ID: "leaf"}, {Kind: memberNode, ID: nodes[1].ID},
			}}
			p.Groups = []RuleGroup{parent, middle, leaf}
			result, err := RenderChannelNodes(nodes, format, *p)
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
				leaf.ID, leaf.Name, leaf.Rules = "leaf", "Leaf", nil
				leaf.Rules = []ChannelRule{}
				parent := RuleGroup{ID: "main", Name: "Parent", Enabled: true, Type: kind, Members: []GroupMember{{Kind: memberGroup, ID: "leaf"}, {Kind: memberNode, ID: nodes[1].ID}}, Rules: []ChannelRule{{ID: "rule", Enabled: true, Kind: "domain", Value: "example.org", Exit: RouteExit{Kind: "group-default"}}}}
				p.Groups = []RuleGroup{parent, leaf}
				result, err := RenderChannelNodes(nodes[1:], format, *p)
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
					rule := root["route"].(map[string]any)["rules"].([]any)[0].(map[string]any)
					if (kind == "select" && rule["action"] != "reject") || (kind == "url-test" && rule["outbound"] != "Parent") {
						t.Fatal(rule)
					}
				} else {
					target := "Parent"
					if kind == "select" {
						target = "REJECT"
					}
					if root["rules"].([]any)[0] != "DOMAIN,example.org,"+target {
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
			p.Groups = append(p.Groups, RuleGroup{ID: "last", Name: "Last", Enabled: true, Type: "select", Members: groupMembers("main"), Rules: []ChannelRule{}})
		}, "cyclic_group_reference"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, p := policyFixture(t, RenderFormatMihomo)
			p.Groups = append(p.Groups, RuleGroup{ID: "child", Name: "Child", Type: "select", Enabled: true, Members: []GroupMember{}, Rules: []ChannelRule{}})
			test.change(p)
			var problem *PolicyError
			if err := ValidateChannelConfig(*p, RenderFormatMihomo); !errors.As(err, &problem) || problem.Code != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

func TestChannelRuleSetNamesAreUniqueIncludingDisabledRules(t *testing.T) {
	_, p := policyFixture(t, RenderFormatMihomo)
	rule := p.Groups[0].Rules[3]
	rule.Enabled = false
	rule.Exit = RouteExit{Kind: "group-default"}
	p.Groups = append(p.Groups, RuleGroup{ID: "other", Name: "Other", Type: "select", Members: []GroupMember{}, Rules: []ChannelRule{rule}})
	var problem *PolicyError
	if err := ValidateChannelConfig(*p, RenderFormatMihomo); !errors.As(err, &problem) || problem.Path != "groups[1].rules[0].remote.name" || problem.Code != "duplicate_name" {
		t.Fatal(err)
	}
	remote := *rule.Remote
	remote.Name = "remote rules"
	p.Groups[1].Rules[0].Remote = &remote
	if err := ValidateChannelConfig(*p, RenderFormatMihomo); err != nil {
		t.Fatal(err)
	}
}

func TestChannelSortIndexUsesSafeSignedIntegers(t *testing.T) {
	for _, index := range []int64{-9007199254740991, -1, 0, 9007199254740991, -9007199254740992, 9007199254740992} {
		_, p := policyFixture(t, RenderFormatMihomo)
		p.Groups[0].Rules[0].SortIndex = index
		err := ValidateChannelConfig(*p, RenderFormatMihomo)
		if valid := index >= -9007199254740991 && index <= 9007199254740991; (err == nil) != valid {
			t.Fatalf("index %d: %v", index, err)
		}
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
			before, err := RenderChannelNodes(nodes, format, *policy)
			if err != nil {
				t.Fatal(err)
			}
			after, err := RenderChannelNodes([]ChannelNode{nodes[2], nodes[1], nodes[0]}, format, *policy)
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
