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

// RenderPolicyNodes consumes an already-authorized, visible catalog. It is the
// sole generation path for modern preview and public delivery, and performs no
// I/O. Missing/hidden dependencies never silently route through another proxy.
func RenderPolicyNodes(nodes []Node, channel RenderChannel, policy *ChannelPolicy) (RenderResult, error) {
	if policy == nil {
		return RenderNodes(nodes, channel)
	}
	if err := ValidateChannelPolicy(policy, channel.Format); err != nil {
		return RenderResult{}, err
	}
	template, err := parseChannelTemplate(policy.Template, channel.Format)
	if err != nil {
		return RenderResult{}, err
	}
	values, ids, err := prepareChannelNodes(nodes, channel, policy)
	if err != nil {
		return RenderResult{}, err
	}
	diagnostics := []RenderDiagnostic{}
	native := []map[string]any{}
	available := map[string]string{}
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
	} else {
		for _, value := range values {
			converted, code := convertMihomo(value)
			if code != "" {
				diagnostics = append(diagnostics, diagnostic(channel.Format, CollectionOutbounds, value.index, code))
				continue
			}
			item := map[string]any{}
			for _, field := range converted {
				item[field.name] = field.value
			}
			native = append(native, item)
		}
	}
	if len(diagnostics) > 0 && policy.Organizer.Incompatible == "error" {
		return RenderResult{}, policyError("policy.organizer.incompatible", "incompatible_nodes")
	}
	nameKey := "tag"
	if channel.Format == RenderFormatMihomo {
		nameKey = "name"
	}
	survivors := map[string]bool{}
	for _, node := range native {
		survivors[node[nameKey].(string)] = true
	}
	for id, name := range ids {
		if survivors[name] {
			available[id] = name
		}
	}
	builder := channelRuleBuilder{format: channel.Format, names: available, groups: map[string]RuleGroup{}, candidates: map[string][]string{}, rules: []any{}, providers: map[string]any{}, sets: []any{}}
	groupNodes := []map[string]any{}
	for _, group := range policy.Groups {
		if group.Enabled {
			builder.groups[group.ID] = group
			builder.candidates[group.ID] = builder.groupCandidates(group)
		}
	}
	for _, group := range policy.Groups {
		if !group.Enabled {
			continue
		}
		if nativeGroup := builder.renderGroup(group); nativeGroup != nil {
			groupNodes = append(groupNodes, nativeGroup)
		}
		for _, rule := range group.Rules {
			if rule.Enabled {
				builder.addRule(rule, &group)
			}
		}
	}
	final := builder.exitName(policy.DefaultExit, nil)
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
		generated["route"] = map[string]any{"rules": builder.rules, "rule_set": builder.sets, "final": final}
	} else {
		builder.rules = append(builder.rules, "MATCH,"+final)
		generated = map[string]any{"proxies": native, "proxy-groups": groupNodes, "rule-providers": builder.providers, "rules": builder.rules}
	}
	content, err := template.render(channel.Format, generated)
	if err != nil {
		return RenderResult{}, err
	}
	if len(content) > MaximumDocumentBytes {
		return RenderResult{}, policyError("policy", "output_too_large")
	}
	media := "application/json"
	if channel.Format == RenderFormatMihomo {
		media = "application/yaml; charset=utf-8"
	}
	return RenderResult{Format: channel.Format, MediaType: media, Content: content, NodeCount: len(native), Diagnostics: diagnostics}, nil
}

func prepareChannelNodes(nodes []Node, channel RenderChannel, p *ChannelPolicy) ([]outbound, map[string]string, error) {
	if len(nodes) > MaximumNodes {
		return nil, nil, policyError("policy.selection", "too_many_nodes")
	}
	filter, err := validateChannel(channel)
	if err != nil {
		return nil, nil, err
	}
	selected, excluded := stringSet(p.Selection.IDs), stringSet(p.Selection.ExcludedIDs)
	chosen := []Node{}
	for _, node := range nodes {
		id := PublicationID(node)
		if excluded[id] || (p.Selection.NewNodePolicy == "exclude" && !selected[id]) {
			continue
		}
		if _, skip := filter.tags[node.Tag]; skip {
			continue
		}
		if _, skip := filter.types[node.Type]; skip {
			continue
		}
		omit := false
		for _, part := range p.Organizer.ExcludeNames {
			if strings.Contains(strings.ToLower(channelNodeName(node)), strings.ToLower(part)) {
				omit = true
				break
			}
		}
		if !omit {
			chosen = append(chosen, node)
		}
	}
	if p.Organizer.Sort == "name" {
		sort.SliceStable(chosen, func(i, j int) bool { return channelNodeName(chosen[i]) < channelNodeName(chosen[j]) })
	} else {
		positions := map[string]int{}
		for i, id := range p.Selection.IDs {
			positions[id] = i
		}
		sort.SliceStable(chosen, func(i, j int) bool {
			a, okA := positions[PublicationID(chosen[i])]
			b, okB := positions[PublicationID(chosen[j])]
			if okA != okB {
				return okA
			}
			return okA && a < b
		})
	}
	used := map[string]bool{"direct": true, "DIRECT": true, "REJECT": true, "GLOBAL": true}
	for _, g := range p.Groups {
		used[g.Name] = true
	}
	names := map[string]string{}
	identical := map[string]string{}
	sourceNames := map[string]string{}
	values := []outbound{}
	sourceIDs := []string{}
	for index, node := range chosen {
		value, err := DecodeDocumentObject(node.Outbound)
		if err != nil {
			return nil, nil, err
		}
		if !publishableOutbound(node.Type) {
			continue
		}
		id := PublicationID(node)
		delete(value, "tag")
		if p.Organizer.Deduplicate {
			key := channelNodeFingerprint(node, value)
			if name := identical[key]; name != "" {
				names[id] = name
				sourceNames[node.SourceID+"\x00"+node.Tag] = name
				continue
			}
		}
		name := p.Organizer.Prefix + channelNodeName(node)
		if strings.ContainsAny(name, "\r\n,\x00") || len(name) > 512 {
			return nil, nil, policyError("policy.selection", "invalid_node_name")
		}
		if used[name] {
			sum := sha256.Sum256([]byte(id))
			name += " · " + hex.EncodeToString(sum[:4])
			for used[name] {
				name += "_"
			}
		}
		used[name] = true
		names[id] = name
		sourceNames[node.SourceID+"\x00"+node.Tag] = name
		if p.Organizer.Deduplicate {
			identical[channelNodeFingerprint(node, value)] = name
		}
		value["tag"] = name
		values = append(values, outbound{collection: CollectionOutbounds, index: index, tag: name, typeID: node.Type, value: value})
		sourceIDs = append(sourceIDs, node.SourceID)
	}
	for i, value := range values {
		if detour, ok := value.value["detour"].(string); ok {
			if name := sourceNames[sourceIDs[i]+"\x00"+detour]; name != "" {
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
func (b *channelRuleBuilder) groupCandidates(group RuleGroup) []string {
	candidates := []string{}
	seen := map[string]bool{}
	for _, key := range group.candidateOrder() {
		var name string
		if id, ok := strings.CutPrefix(key, "node:"); ok {
			name = b.names[id]
		} else if kind, ok := strings.CutPrefix(key, "builtin:"); ok {
			name = b.exitName(RouteExit{Kind: kind}, nil)
		}
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
			defaultName := b.exitName(group.DefaultExit, nil)
			if defaultName == "REJECT" {
				// Rejection is a route action in current sing-box. groupExit
				// rejects references to this group until its fixed exit returns.
				defaultName = candidates[0]
			}
			result["default"] = defaultName
		} else {
			result["type"] = "urltest"
		}
	} else {
		if kind == "select" {
			defaultName := b.exitName(group.DefaultExit, nil)
			ordered := []string{defaultName}
			for _, name := range candidates {
				if name != defaultName {
					ordered = append(ordered, name)
				}
			}
			candidates = ordered
		}
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
	// Auto groups may use remaining candidates. An unavailable manual fixed
	// default must retain the existing fail-closed routing behavior.
	if group.Type == "select" && b.format == RenderFormatSingBox && (group.DefaultExit.Kind == "reject" || (group.DefaultExit.Kind == "node" && b.names[group.DefaultExit.ID] == "")) {
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
		name := "rules-" + group.ID + "-" + rule.ID
		if b.format == RenderFormatSingBox {
			b.sets = append(b.sets, map[string]any{"type": "remote", "tag": name, "format": remote.Format, "url": EffectiveRuleURL(remote.URL, remote.Accelerated), "update_interval": fmt.Sprintf("%ds", remote.UpdateInterval), "download_detour": "direct"})
		} else {
			b.providers[name] = map[string]any{"type": "http", "behavior": remote.Behavior, "format": remote.Format, "url": EffectiveRuleURL(remote.URL, remote.Accelerated), "interval": remote.UpdateInterval}
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

func channelNodeName(node Node) string {
	if node.OriginTag != "" {
		return node.OriginTag
	}
	return node.Tag
}

func channelNodeFingerprint(node Node, value map[string]any) string {
	raw, _ := json.Marshal(value)
	// A detour name is scoped to its source. Identical spellings in different
	// subscriptions can resolve to entirely different proxy chains.
	if _, exists := value["detour"]; exists {
		raw = append([]byte(node.SourceID+"\x00"), raw...)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
