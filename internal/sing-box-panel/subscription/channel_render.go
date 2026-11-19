// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// ChannelNode is a pool node as a channel renders it.
type ChannelNode struct {
	ID string
	// Name is the node's display name.
	Name     string
	Protocol string
	// SourceID and OriginName let a source node's detour name another node
	// of its source.
	SourceID   string
	OriginName string
	// Outbound is the node's sing-box outbound without its tag.
	Outbound json.RawMessage
}

// RenderChannelNodes renders a channel from the pool nodes the caller may
// publish, in pool order. Only the nodes the channel's enabled groups name
// as members, rule exits or the default exit are published. It performs no
// I/O, and a missing dependency never silently routes through another proxy.
func RenderChannelNodes(nodes []ChannelNode, format RenderFormat, config ChannelConfig) (RenderResult, error) {
	if err := ValidateChannelConfig(config, format); err != nil {
		return RenderResult{}, err
	}
	template, err := parseChannelTemplate(config.Template, format)
	if err != nil {
		return RenderResult{}, err
	}
	values, ids, err := prepareChannelNodes(nodes, format, config)
	if err != nil {
		return RenderResult{}, err
	}
	channel := RenderChannel{Format: format}
	diagnostics := []RenderDiagnostic{}
	native := []map[string]any{}
	loonNodes := []loonNode{}
	available := map[string]string{}
	endpoints := []map[string]any{}
	if channel.Format == RenderFormatSingBox {
		result, issues := renderSingBox(startupNodes{values: values}, nil)
		diagnostics = append(diagnostics, issues...)
		root, err := DecodeDocumentObject(result.Content)
		if err != nil {
			return RenderResult{}, err
		}
		for _, value := range root["outbounds"].([]any) {
			native = append(native, value.(map[string]any))
		}
		if values, ok := root["endpoints"].([]any); ok {
			for _, value := range values {
				endpoints = append(endpoints, value.(map[string]any))
			}
		}
	} else if channel.Format == RenderFormatLoon {
		for _, value := range values {
			converted, code := convertLoon(value)
			if code != "" {
				diagnostics = append(diagnostics, diagnostic(channel.Format, value.collection, value.index, code))
				continue
			}
			loonNodes = append(loonNodes, converted)
		}
	} else {
		for _, value := range values {
			converted, code := convertMihomo(value)
			if code != "" {
				diagnostics = append(diagnostics, diagnostic(channel.Format, value.collection, value.index, code))
				continue
			}
			item := map[string]any{}
			for _, field := range converted {
				item[field.name] = field.value
			}
			native = append(native, item)
		}
	}
	if len(diagnostics) > 0 && config.IncompatibleNodes == "error" {
		return RenderResult{}, policyError("incompatible_nodes", "incompatible_nodes")
	}
	nameKey := "tag"
	if channel.Format == RenderFormatMihomo {
		nameKey = "name"
	}
	survivors := map[string]bool{}
	for _, node := range append(append([]map[string]any{}, native...), endpoints...) {
		survivors[node[nameKey].(string)] = true
	}
	for _, node := range loonNodes {
		survivors[node.name] = true
	}
	for id, name := range ids {
		if survivors[name] {
			available[id] = name
		}
	}
	builder := channelRuleBuilder{format: channel.Format, names: available, groups: map[string]RuleGroup{}, candidates: map[string][]string{}, rules: []any{}, providers: map[string]any{}, sets: []any{}}
	groupNodes := builder.renderGroups(config.Groups)
	orderedRules := []orderedChannelRule{}
	for _, group := range config.Groups {
		if !group.Enabled {
			continue
		}
		for _, rule := range group.Rules {
			if rule.Enabled {
				orderedRules = append(orderedRules, orderedChannelRule{rule: rule, group: group})
			}
		}
	}
	sort.SliceStable(orderedRules, func(i, j int) bool {
		a, b := orderedRules[i], orderedRules[j]
		if a.rule.SortIndex != b.rule.SortIndex {
			return a.rule.SortIndex < b.rule.SortIndex
		}
		return a.rule.sortName() < b.rule.sortName()
	})
	for _, item := range orderedRules {
		builder.addRule(item.rule, &item.group)
	}
	final := builder.exitName(config.DefaultExit, nil)
	generated := map[string]any{}
	if channel.Format == RenderFormatSingBox {
		outbounds := append([]map[string]any{}, native...)
		outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})
		outbounds = append(outbounds, groupNodes...)
		if final == "REJECT" {
			builder.rules = append(builder.rules, map[string]any{"action": "reject"})
			final = "direct"
		}
		generated["outbounds"] = outbounds
		if len(endpoints) > 0 {
			generated["endpoints"] = endpoints
		}
		generated["route"] = map[string]any{"rules": builder.rules, "rule_set": builder.sets, "final": final}
	} else if channel.Format == RenderFormatMihomo {
		builder.rules = append(builder.rules, "MATCH,"+final)
		generated = map[string]any{"proxies": native, "proxy-groups": groupNodes, "rule-providers": builder.providers, "rules": builder.rules}
	}
	var content []byte
	if channel.Format == RenderFormatLoon {
		content = renderLoonChannel(template.loon, loonNodes, groupNodes, builder.rules, builder.loonRemote, final)
	} else {
		content, err = template.render(channel.Format, generated)
	}
	if err != nil {
		return RenderResult{}, err
	}
	if len(content) > MaximumDocumentBytes {
		return RenderResult{}, policyError("config", "output_too_large")
	}
	media := "application/json"
	if channel.Format == RenderFormatMihomo {
		media = "application/yaml; charset=utf-8"
	} else if channel.Format == RenderFormatLoon {
		media = "text/plain; charset=utf-8"
	}
	carried := make(map[string]bool, len(available))
	for id := range available {
		carried[id] = true
	}
	return RenderResult{Format: channel.Format, MediaType: media, Content: content, NodeCount: len(native) + len(endpoints) + len(loonNodes), Diagnostics: diagnostics, PreviewDiagnostics: previewDiagnostics(diagnostics, values), carried: carried}, nil
}

// publishedNodeIDs returns the pool nodes a channel publishes: those its
// enabled groups name as members or as exits of their enabled rules, and its
// default exit.
func publishedNodeIDs(config ChannelConfig) map[string]bool {
	published := map[string]bool{}
	for _, group := range config.Groups {
		if !group.Enabled {
			continue
		}
		for _, member := range group.Members {
			if member.Kind == memberNode {
				published[member.ID] = true
			}
		}
		for _, rule := range group.Rules {
			if rule.Enabled && rule.Exit.Kind == "node" {
				published[rule.Exit.ID] = true
			}
		}
	}
	if config.DefaultExit.Kind == "node" {
		published[config.DefaultExit.ID] = true
	}
	return published
}

func prepareChannelNodes(nodes []ChannelNode, format RenderFormat, config ChannelConfig) ([]outbound, map[string]string, error) {
	if len(nodes) > MaximumNodes {
		return nil, nil, policyError("groups", "too_many_nodes")
	}
	published := publishedNodeIDs(config)
	used := map[string]bool{"direct": true, "DIRECT": true, "REJECT": true, "GLOBAL": true}
	if format == RenderFormatLoon {
		used["PROXY"] = true
	}
	for _, g := range config.Groups {
		used[g.Name] = true
	}
	names := map[string]string{}
	originNames := map[string]string{}
	values := []outbound{}
	chosen := []ChannelNode{}
	for _, node := range nodes {
		if !published[node.ID] {
			continue
		}
		value, err := DecodeDocumentObject(node.Outbound)
		if err != nil {
			return nil, nil, err
		}
		if !publishableOutbound(node.Protocol) {
			continue
		}
		delete(value, "tag")
		name := node.Name
		if strings.ContainsAny(name, "\r\n,\x00") || len(name) > 512 {
			return nil, nil, policyError("nodes", "invalid_node_name")
		}
		if format == RenderFormatLoon && !validLoonPolicyName(name) {
			return nil, nil, policyError("nodes", "invalid_node_name")
		}
		if used[name] {
			sum := sha256.Sum256([]byte(node.ID))
			name += " · " + hex.EncodeToString(sum[:4])
			for used[name] {
				name += "_"
			}
		}
		used[name] = true
		names[node.ID] = name
		if node.SourceID != "" {
			key := node.SourceID + "\x00" + node.OriginName
			if _, taken := originNames[key]; !taken {
				originNames[key] = name
			}
		}
		value["tag"] = name
		collection := CollectionOutbounds
		if node.Protocol == "wireguard" && publishableEndpoint(node.Protocol) && value["peers"] != nil {
			collection = CollectionEndpoints
		}
		values = append(values, outbound{collection: collection, index: len(values), nodeID: node.ID, tag: name, typeID: node.Protocol, value: value})
		chosen = append(chosen, node)
	}
	for i, value := range values {
		if detour, ok := value.value["detour"].(string); ok {
			if name := originNames[chosen[i].SourceID+"\x00"+detour]; name != "" && chosen[i].SourceID != "" {
				value.value["detour"] = name
			} else {
				value.value["detour"] = "\x00unresolved-detour"
			}
		}
	}
	return values, names, nil
}

type channelRuleBuilder struct {
	format     RenderFormat
	names      map[string]string
	groups     map[string]RuleGroup
	candidates map[string][]string
	rules      []any
	providers  map[string]any
	sets       []any
	loonRemote []string
}

func (b *channelRuleBuilder) exitName(exit RouteExit, group *RuleGroup) string {
	switch exit.Kind {
	case "direct":
		if b.format == RenderFormatSingBox {
			return "direct"
		}
		return "DIRECT"
	case "node":
		if name := b.names[exit.ID]; name != "" {
			return name
		}
	case "group":
		if g, ok := b.groups[exit.ID]; ok {
			return b.groupExit(g)
		}
	case "group-default":
		if group != nil {
			return b.groupExit(*group)
		}
	}
	return "REJECT"
}

// Validation establishes an acyclic graph before generation. Resolve children
// before parents, independently of the editing order, and emit each group once.
func (b *channelRuleBuilder) renderGroups(groups []RuleGroup) []map[string]any {
	for _, group := range groups {
		if group.Enabled {
			b.groups[group.ID] = group
		}
	}
	result := []map[string]any{}
	var visit func(RuleGroup)
	visit = func(group RuleGroup) {
		if _, done := b.candidates[group.ID]; done {
			return
		}
		for _, member := range group.Members {
			if member.Kind == memberGroup {
				visit(b.groups[member.ID])
			}
		}
		b.candidates[group.ID] = b.groupCandidates(group)
		if value := b.renderGroup(group); value != nil {
			result = append(result, value)
		}
	}
	for _, group := range groups {
		if group.Enabled {
			visit(group)
		}
	}
	return result
}

func (b *channelRuleBuilder) candidateName(member GroupMember) string {
	switch member.Kind {
	case memberNode:
		return b.names[member.ID]
	case memberBuiltin:
		return b.exitName(RouteExit{Kind: member.ID}, nil)
	case memberGroup:
		if group, exists := b.groups[member.ID]; exists && b.groupExit(group) != "REJECT" {
			return group.Name
		}
	}
	return ""
}

func (b *channelRuleBuilder) groupCandidates(group RuleGroup) []string {
	candidates := []string{}
	seen := map[string]bool{}
	for _, member := range group.Members {
		name := b.candidateName(member)
		if name != "" && !seen[name] {
			candidates = append(candidates, name)
			seen[name] = true
		}
	}
	return candidates
}

func (b *channelRuleBuilder) renderGroup(group RuleGroup) map[string]any {
	candidates := b.candidates[group.ID]
	if len(candidates) == 0 {
		// Native empty groups can imply DIRECT or be invalid. Their references
		// are rendered as rejection instead, without an unusable group.
		return nil
	}
	kind := group.Type
	var result map[string]any
	if b.format == RenderFormatSingBox {
		result = map[string]any{"tag": group.Name, "outbounds": candidates}
		if kind == "select" {
			result["type"] = "selector"
			result["default"] = candidates[0]
		} else {
			result["type"] = "urltest"
		}
	} else {
		result = map[string]any{"name": group.Name, "type": kind, "proxies": candidates}
	}
	if kind != "select" {
		check := group.healthCheck()
		result["url"] = check.URL
		if b.format == RenderFormatSingBox {
			result["interval"] = fmt.Sprintf("%ds", check.Interval)
			// sing-box requires its idle timeout to be at least the interval.
			if check.Interval > 1800 {
				result["idle_timeout"] = fmt.Sprintf("%ds", check.Interval)
			}
		} else {
			result["interval"] = check.Interval
		}
		if kind == "url-test" {
			result["tolerance"] = check.Tolerance
		}
	}
	return result
}

func (b *channelRuleBuilder) groupExit(group RuleGroup) string {
	if len(b.candidates[group.ID]) == 0 {
		return "REJECT"
	}
	// A missing first manual candidate must not silently select another path.
	// Keep routing fail-closed without inserting or reordering candidates.
	if group.Type == "select" && len(group.Members) > 0 && b.candidateName(group.Members[0]) == "" {
		return "REJECT"
	}
	return group.Name
}
func (b *channelRuleBuilder) addRule(rule ChannelRule, group *RuleGroup) {
	target := b.exitName(rule.Exit, group)
	value := rule.Value
	kind := rule.Kind
	if kind == "ip_cidr" {
		if addr, err := netip.ParseAddr(value); err == nil {
			value = netip.PrefixFrom(addr, addr.BitLen()).String()
		}
	}
	if kind == "remote" {
		remote := rule.Remote
		if b.format == RenderFormatLoon {
			b.loonRemote = append(b.loonRemote, EffectiveRuleURL(remote.URL, remote.Accelerated)+",policy="+target+",tag="+loonOptionValue(remote.Name)+",enabled=true")
			return
		}
		name := remote.Name
		if b.format == RenderFormatSingBox {
			b.sets = append(b.sets, map[string]any{"type": "remote", "tag": name, "format": remote.Format, "url": EffectiveRuleURL(remote.URL, remote.Accelerated), "http_client": map[string]any{"detour": "direct"}})
		} else {
			b.providers[name] = map[string]any{"type": "http", "behavior": remote.Behavior, "format": remote.Format, "url": EffectiveRuleURL(remote.URL, remote.Accelerated)}
		}
		value = name
	}
	if b.format == RenderFormatSingBox {
		if kind == "remote" {
			kind = "rule_set"
		}
		entry := map[string]any{kind: []string{value}, "action": "route", "outbound": target}
		if target == "REJECT" {
			delete(entry, "outbound")
			entry["action"] = "reject"
		}
		b.rules = append(b.rules, entry)
	} else {
		prefix := map[string]string{"domain": "DOMAIN", "domain_suffix": "DOMAIN-SUFFIX", "domain_keyword": "DOMAIN-KEYWORD", "ip_cidr": "IP-CIDR", "remote": "RULE-SET"}[kind]
		if kind == "ip_cidr" {
			if cidr, err := netip.ParsePrefix(value); err == nil && cidr.Addr().Is6() {
				prefix = "IP-CIDR6"
			}
		}
		b.rules = append(b.rules, prefix+","+value+","+target)
	}
}

type orderedChannelRule struct {
	rule  ChannelRule
	group RuleGroup
}

func (rule ChannelRule) sortName() string {
	if rule.Remote != nil {
		return rule.Remote.Name
	}
	return rule.Value
}
