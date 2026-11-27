// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
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
// publish, in pool order, and the rule sets its exit rules route by. Only
// the nodes the channel's enabled groups name as members or match, its
// enabled exit rules' exits or its final exit are published. It performs no
// I/O, and a missing dependency never silently routes through another proxy.
func RenderChannelNodes(nodes []ChannelNode, format RenderFormat, config ChannelConfig, ruleSets map[string]ChannelRuleSet) (RenderResult, error) {
	if err := ValidateChannelConfig(config, format); err != nil {
		return RenderResult{}, err
	}
	if err := validateChannelRuleSets(config, format, ruleSets); err != nil {
		return RenderResult{}, err
	}
	template, err := parseChannelTemplate(config.Template, format)
	if err != nil {
		return RenderResult{}, err
	}
	config = expandGroupMatches(config, nodes)
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
	builder := channelRuleBuilder{
		format: channel.Format, names: available, groups: map[string]RuleGroup{}, candidates: map[string][]string{},
		rules: []any{}, providers: map[string]any{}, sets: []any{}, remotes: map[string]remoteSource{},
	}
	groupNodes := builder.renderGroups(config.Groups)
	for _, route := range config.Routes {
		if !route.Enabled {
			continue
		}
		target := builder.exitName(route.Exit)
		if route.Rule != nil {
			builder.addRule(*route.Rule, target)
			continue
		}
		for _, rule := range ruleSets[route.RuleSetID].Rules {
			builder.addRule(rule, target)
		}
	}
	builder.rules = append(leadingRules(channel.Format), builder.rules...)
	final := builder.exitName(config.DefaultExit)
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
// enabled groups name as members, its enabled exit rules' exits and its
// final exit. Matched nodes are members once expandGroupMatches has run.
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
	}
	for _, route := range config.Routes {
		if route.Enabled && route.Exit.Kind == "node" {
			published[route.Exit.ID] = true
		}
	}
	if config.DefaultExit.Kind == "node" {
		published[config.DefaultExit.ID] = true
	}
	return published
}

// expandGroupMatches returns config with each enabled group's matched pool
// nodes appended to its members, in pool order and each once. config itself
// is not changed.
func expandGroupMatches(config ChannelConfig, nodes []ChannelNode) ChannelConfig {
	expanded := config
	expanded.Groups = make([]RuleGroup, len(config.Groups))
	for index, group := range config.Groups {
		expanded.Groups[index] = group
		if !group.Enabled || group.Match == nil {
			continue
		}
		matched := group.Match.nodes(nodes)
		if len(matched) == 0 {
			continue
		}
		members := append([]GroupMember{}, group.Members...)
		seen := make(map[string]bool, len(members))
		for _, member := range members {
			if member.Kind == memberNode {
				seen[member.ID] = true
			}
		}
		for _, id := range matched {
			if !seen[id] {
				seen[id] = true
				members = append(members, GroupMember{Kind: memberNode, ID: id})
			}
		}
		expanded.Groups[index].Members = members
	}
	return expanded
}

// nodes returns the IDs of the pool nodes match takes, in pool order.
func (match GroupMatch) nodes(pool []ChannelNode) []string {
	if len(match.Include) == 0 && len(match.Exclude) == 0 && len(match.Origins) == 0 && len(match.Protocols) == 0 {
		return nil
	}
	filter, err := SourceFilter{Include: match.Include, Exclude: match.Exclude}.compile()
	if err != nil {
		return nil
	}
	ids := []string{}
	for _, node := range pool {
		origin := originFleet
		if node.SourceID != "" {
			origin = originPrefixSource + node.SourceID
		}
		if (len(match.Origins) == 0 || slices.Contains(match.Origins, origin)) &&
			(len(match.Protocols) == 0 || slices.Contains(match.Protocols, node.Protocol)) && filter.keep(node.Name) {
			ids = append(ids, node.ID)
		}
	}
	return ids
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
	// remotes holds the source of each remote rule set tag emitted so far.
	remotes map[string]remoteSource
}

// remoteSource is what a client fetches for a remote rule set.
type remoteSource struct {
	url, format, behavior string
}

func (b *channelRuleBuilder) exitName(exit RouteExit) string {
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
		return b.exitName(RouteExit{Kind: member.ID})
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

// remoteTag returns the tag of a remote rule set and whether it is new. Rule
// sets that share a name and source share one tag; a later one with the
// same name but another source takes the name with the next free suffix.
func (b *channelRuleBuilder) remoteTag(remote RemoteRuleSet) (string, bool) {
	source := remoteSource{url: EffectiveRuleURL(remote.URL, remote.Accelerated), format: remote.Format, behavior: remote.Behavior}
	tag := remote.Name
	for suffix := 2; ; suffix++ {
		existing, taken := b.remotes[tag]
		if !taken {
			b.remotes[tag] = source
			return tag, true
		}
		if existing == source {
			return tag, false
		}
		tag = fmt.Sprintf("%s-%d", remote.Name, suffix)
	}
}

func (b *channelRuleBuilder) addRule(rule SubscriptionRule, target string) {
	value := rule.Value
	kind := rule.Kind
	if kind == "ip_cidr" {
		if addr, err := netip.ParseAddr(value); err == nil {
			value = netip.PrefixFrom(addr, addr.BitLen()).String()
		}
	}
	if kind == "remote" {
		remote := rule.Remote
		name, added := b.remoteTag(*remote)
		url := EffectiveRuleURL(remote.URL, remote.Accelerated)
		if b.format == RenderFormatLoon {
			// A Loon remote rule carries its own policy; a later one with the
			// same tag would never match first.
			if added {
				b.loonRemote = append(b.loonRemote, url+",policy="+target+",tag="+loonOptionValue(name)+",enabled=true")
			}
			return
		}
		if added && b.format == RenderFormatSingBox {
			b.sets = append(b.sets, map[string]any{"type": "remote", "tag": name, "format": remote.Format, "url": url, "http_client": map[string]any{"detour": "direct"}})
		} else if added {
			b.providers[name] = map[string]any{"type": "http", "behavior": remote.Behavior, "format": remote.Format, "url": url}
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
		line := prefix + "," + value + "," + target
		if kind == "ip_cidr" {
			if cidr, err := netip.ParsePrefix(value); err == nil && cidr.Addr().Is6() {
				line = "IP-CIDR6," + value + "," + target
			}
			// Matched by the connection's own address: the client never
			// resolves a domain locally only to test this rule.
			line += ",no-resolve"
		} else if kind == "remote" && rule.Remote.Behavior != "domain" {
			line += ",no-resolve"
		}
		b.rules = append(b.rules, line)
	}
}

// stunPorts are the UDP ports WebRTC reaches STUN servers on: the standard
// port and Google's range.
const stunPorts = "3478/19302-19309"

// leadingRules come before a channel's exit rules. sing-box sniffs each
// connection so one to an address still matches by its domain, answers DNS
// from its DNS module, rejects STUN so WebRTC cannot learn the device's own
// address, and keeps private addresses direct. mihomo cannot recognize STUN
// and rejects it by its UDP ports. Loon refuses STUN by its own setting.
func leadingRules(format RenderFormat) []any {
	switch format {
	case RenderFormatSingBox:
		return []any{
			map[string]any{"action": "sniff"},
			map[string]any{"protocol": []string{"dns"}, "action": "hijack-dns"},
			map[string]any{"protocol": []string{"stun"}, "action": "reject"},
			map[string]any{"ip_is_private": true, "action": "route", "outbound": "direct"},
		}
	case RenderFormatMihomo:
		return []any{"AND,((NETWORK,UDP),(DST-PORT," + stunPorts + ")),REJECT"}
	}
	return []any{}
}
