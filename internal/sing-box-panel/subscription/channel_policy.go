// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// PolicyError contains only a field path and a fixed reason, never submitted
// values, URLs or template content. It is safe for authenticated validation UI.
type PolicyError struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("invalid subscription channel: %s: %s", e.Path, e.Code)
}
func (e *PolicyError) Unwrap() error      { return ErrInvalidChannel }
func policyError(path, code string) error { return &PolicyError{Path: path, Code: code} }

const (
	maximumChannelRoutes  = 5000
	maximumRuleSetRules   = 5000
	maximumMatchOrigins   = 256
	maximumMatchProtocols = 32
)

// ValidateChannelConfig checks a channel configuration for one output
// format. References to pool nodes and rule sets are checked separately.
func ValidateChannelConfig(config ChannelConfig, format RenderFormat) error {
	if format != RenderFormatSingBox && format != RenderFormatMihomo && format != RenderFormatLoon {
		return policyError("format", "unsupported_client")
	}
	if !oneOf(config.IncompatibleNodes, "skip", "error") {
		return policyError("incompatible_nodes", "invalid_value")
	}
	if len(config.Groups) > 256 {
		return policyError("groups", "too_many_groups")
	}
	groups, names := map[string]bool{}, map[string]bool{}
	for i, group := range config.Groups {
		path := fmt.Sprintf("groups[%d]", i)
		if !policyID(group.ID) || groups[group.ID] {
			return policyError(path+".id", "invalid_or_duplicate_id")
		}
		groups[group.ID] = true
		if !policyName(group.Name) || names[group.Name] || oneOf(group.Name, "direct", "DIRECT", "REJECT", "GLOBAL") {
			return policyError(path+".name", "invalid_or_duplicate_name")
		}
		if format == RenderFormatLoon && (!validLoonPolicyName(group.Name) || group.Name == "PROXY") {
			return policyError(path+".name", "invalid_or_duplicate_name")
		}
		names[group.Name] = true
	}
	enabled := map[string]bool{}
	for _, group := range config.Groups {
		if group.Enabled {
			enabled[group.ID] = true
		}
	}
	for i, group := range config.Groups {
		path := fmt.Sprintf("groups[%d]", i)
		if err := validateGroupOptions(group, format, path); err != nil {
			return err
		}
		if err := validateGroupMembers(group, groups, format, path); err != nil {
			return err
		}
		if err := validateGroupMatch(group.Match, path+".match"); err != nil {
			return err
		}
	}
	if err := validateGroupReferences(config.Groups); err != nil {
		return err
	}
	if len(config.Routes) > maximumChannelRoutes {
		return policyError("routes", "too_many_routes")
	}
	routes, remoteNames := map[string]bool{}, map[string]bool{}
	for i, route := range config.Routes {
		path := fmt.Sprintf("routes[%d]", i)
		if !policyID(route.ID) || routes[route.ID] {
			return policyError(path+".id", "invalid_or_duplicate_id")
		}
		routes[route.ID] = true
		if (route.RuleSetID == "") == (route.Rule == nil) {
			return policyError(path, "exactly_one_of_rule_set_id_rule")
		}
		if route.Rule != nil {
			if err := validateRule(*route.Rule, format, path+".rule", remoteNames); err != nil {
				return err
			}
		} else if !policyID(route.RuleSetID) {
			return policyError(path+".rule_set_id", "invalid_value")
		}
		if err := validateExit(route.Exit, enabled, path+".exit"); err != nil {
			return err
		}
	}
	if err := validateExit(config.DefaultExit, enabled, "default_exit"); err != nil {
		return err
	}
	_, err := parseChannelTemplate(config.Template, format)
	return err
}

// validateRuleSetRules checks a rule set's rules for its client.
func validateRuleSetRules(rules []SubscriptionRule, format RenderFormat, path string) error {
	if len(rules) > maximumRuleSetRules {
		return policyError(path, "too_many_rules")
	}
	remoteNames := map[string]bool{}
	for i, rule := range rules {
		if err := validateRule(rule, format, fmt.Sprintf("%s[%d]", path, i), remoteNames); err != nil {
			return err
		}
	}
	return nil
}

// validateRule checks one rule for a client. remoteNames holds the remote
// rule set names its list already uses, including disabled entries.
func validateRule(rule SubscriptionRule, format RenderFormat, path string, remoteNames map[string]bool) error {
	switch rule.Kind {
	case "domain", "domain_suffix", "domain_keyword":
		if !policyName(rule.Value) || strings.ContainsAny(rule.Value, " /:@") || rule.Remote != nil || (format == RenderFormatLoon && !validLoonAtom(rule.Value)) {
			return policyError(path+".value", "invalid_domain")
		}
	case "ip_cidr":
		_, prefixErr := netip.ParsePrefix(rule.Value)
		_, addrErr := netip.ParseAddr(rule.Value)
		if len(rule.Value) > 128 || prefixErr != nil && addrErr != nil {
			return policyError(path+".value", "invalid_ip_cidr")
		}
		if rule.Remote != nil {
			return policyError(path+".remote", "unexpected_field")
		}
	case "remote":
		if rule.Remote == nil || rule.Value != "" {
			return policyError(path+".remote", "required")
		}
		if err := validateRemoteRuleSet(*rule.Remote, format, path+".remote"); err != nil {
			return err
		}
		if remoteNames[rule.Remote.Name] {
			return policyError(path+".remote.name", "duplicate_name")
		}
		remoteNames[rule.Remote.Name] = true
	default:
		return policyError(path+".kind", "unsupported_rule")
	}
	return nil
}

// validateGroupMatch checks a group's match: name rules as a source's
// filter, origins by their format and protocols by their length.
func validateGroupMatch(match *GroupMatch, path string) error {
	if match == nil {
		return nil
	}
	if err := (SourceFilter{Include: match.Include, Exclude: match.Exclude}).Validate(); err != nil {
		var field *SourceFieldError
		if errors.As(err, &field) {
			return policyError(path+"."+strings.TrimPrefix(field.Path, "filter."), field.Code)
		}
		return err
	}
	if len(match.Origins) > maximumMatchOrigins {
		return policyError(path+".origins", "too_many_origins")
	}
	for i, origin := range match.Origins {
		if source, ok := strings.CutPrefix(origin, originPrefixSource); origin != originFleet && (!ok || !policyID(source)) {
			return policyError(fmt.Sprintf("%s.origins[%d]", path, i), "invalid_value")
		}
	}
	if len(match.Protocols) > maximumMatchProtocols {
		return policyError(path+".protocols", "too_many_protocols")
	}
	for i, protocol := range match.Protocols {
		if protocol == "" || len(protocol) > 32 || strings.IndexFunc(protocol, unicode.IsControl) >= 0 {
			return policyError(fmt.Sprintf("%s.protocols[%d]", path, i), "invalid_value")
		}
	}
	return nil
}

const (
	originFleet        = "fleet"
	originPrefixSource = "source:"
)

// ChannelRuleSet is a rule set as a channel renders it.
type ChannelRuleSet struct {
	Client RenderFormat
	Rules  []SubscriptionRule
}

// validateChannelRuleSets checks the rule sets config routes by against
// ruleSets: each exists, belongs to the channel's client and holds valid
// rules, and the enabled exit rules hold at most store.MaximumChannelRules
// rules together.
func validateChannelRuleSets(config ChannelConfig, format RenderFormat, ruleSets map[string]ChannelRuleSet) error {
	count := 0
	checked := map[string]bool{}
	for i, route := range config.Routes {
		if route.RuleSetID == "" {
			if route.Enabled {
				count++
			}
			continue
		}
		path := fmt.Sprintf("routes[%d].rule_set_id", i)
		set, ok := ruleSets[route.RuleSetID]
		if !ok {
			return policyError(path, "rule_set_not_found")
		}
		if set.Client != format {
			return policyError(path, "rule_set_client_mismatch")
		}
		if !checked[route.RuleSetID] {
			checked[route.RuleSetID] = true
			if err := validateRuleSetRules(set.Rules, format, fmt.Sprintf("routes[%d].rule_set.rules", i)); err != nil {
				return err
			}
		}
		if route.Enabled {
			count += len(set.Rules)
		}
	}
	if count > store.MaximumChannelRules {
		return policyError("routes", "too_many_rules")
	}
	return nil
}

// validateGroupMembers checks a group's candidates: each once, built-in
// exits the client supports and groups of the channel other than itself.
func validateGroupMembers(group RuleGroup, groups map[string]bool, format RenderFormat, path string) error {
	if len(group.Members) > MaximumNodes+2 {
		return policyError(path+".members", "too_many_members")
	}
	seen := map[GroupMember]bool{}
	for index, member := range group.Members {
		memberPath := fmt.Sprintf("%s.members[%d]", path, index)
		if seen[member] {
			return policyError(memberPath, "duplicate_member")
		}
		seen[member] = true
		switch member.Kind {
		case memberNode:
			if !policyID(member.ID) {
				return policyError(memberPath+".id", "invalid_value")
			}
		case memberBuiltin:
			if !oneOf(member.ID, "direct", "reject") {
				return policyError(memberPath+".id", "invalid_value")
			}
			if format == RenderFormatSingBox && member.ID == "reject" {
				return policyError(memberPath, "unsupported_client")
			}
			if format == RenderFormatLoon && group.Type != "select" {
				return policyError(memberPath, "unsupported_client")
			}
		case memberGroup:
			if !groups[member.ID] {
				return policyError(memberPath+".id", "group_not_found")
			}
			if member.ID == group.ID {
				return policyError(memberPath+".id", "cyclic_group_reference")
			}
		default:
			return policyError(memberPath+".kind", "invalid_value")
		}
	}
	return nil
}

func validateGroupOptions(group RuleGroup, format RenderFormat, path string) error {
	if !oneOf(group.Type, "select", "url-test", "fallback") {
		return policyError(path+".type", "invalid_value")
	}
	if format == RenderFormatSingBox && group.Type == "fallback" {
		return policyError(path+".type", "unsupported_client")
	}
	if group.HealthCheck != nil {
		h := group.HealthCheck
		u, err := url.Parse(h.URL)
		if err != nil || len(h.URL) > 4096 || u.Hostname() == "" || !oneOf(u.Scheme, "http", "https") || u.User != nil || u.Fragment != "" || strings.ContainsAny(h.URL, "\r\n\t ") {
			return policyError(path+".health_check.url", "invalid_url")
		}
		if format == RenderFormatLoon && !validLoonAtom(h.URL) {
			return policyError(path+".health_check.url", "invalid_url")
		}
		if h.Interval < 60 || h.Interval > 86400 || h.Tolerance < 0 || h.Tolerance > 65535 {
			return policyError(path+".health_check", "invalid_value")
		}
	}
	return nil
}

// Check the entire graph, including disabled groups, so enabling a group cannot
// introduce a latent cycle. References must always target an enabled group.
func validateGroupReferences(groups []RuleGroup) error {
	indices := make(map[string]int, len(groups))
	for i, group := range groups {
		indices[group.ID] = i
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(int) error
	visit = func(i int) error {
		group := groups[i]
		if visited[group.ID] {
			return nil
		}
		visiting[group.ID] = true
		for j, member := range group.Members {
			if member.Kind != memberGroup {
				continue
			}
			path := fmt.Sprintf("groups[%d].members[%d]", i, j)
			index, ok := indices[member.ID]
			if !ok || !groups[index].Enabled {
				return policyError(path, "group_not_enabled")
			}
			if visiting[member.ID] {
				return policyError(path, "cyclic_group_reference")
			}
			if err := visit(index); err != nil {
				return err
			}
		}
		delete(visiting, group.ID)
		visited[group.ID] = true
		return nil
	}
	for i := range groups {
		if err := visit(i); err != nil {
			return err
		}
	}
	return nil
}

func policyID(s string) bool {
	return s != "" && len(s) <= 128 && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == ':')
	}) < 0
}
func policyName(s string) bool {
	return s != "" && len(s) <= 128 && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0 && !strings.Contains(s, ",")
}
func validateExit(exit RouteExit, groups map[string]bool, path string) error {
	switch exit.Kind {
	case "direct", "reject":
		if exit.ID != "" {
			return policyError(path, "unexpected_id")
		}
	case "node":
		if !policyID(exit.ID) {
			return policyError(path, "invalid_value")
		}
	case "group":
		if !groups[exit.ID] {
			return policyError(path, "group_not_enabled")
		}
	default:
		return policyError(path, "invalid_exit")
	}
	return nil
}
func validateRemoteRuleSet(r RemoteRuleSet, format RenderFormat, path string) error {
	if !policyName(r.Name) {
		return policyError(path+".name", "invalid_value")
	}
	u, err := url.Parse(r.URL)
	if err != nil || !oneOf(u.Scheme, "https", "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(r.URL) > 4096 || strings.ContainsAny(r.URL, "\r\n\t ") {
		return policyError(path+".url", "invalid_url")
	}
	if r.Accelerated && !CanAccelerateRuleURL(r.URL) {
		return policyError(path+".accelerated", "unsupported_url")
	}
	if format == RenderFormatLoon {
		if !validLoonAtom(r.URL) {
			return policyError(path+".url", "invalid_url")
		}
		if r.Format != "loon" || r.Behavior != "" {
			return policyError(path+".format", "incompatible_format")
		}
		return nil
	}
	if format == RenderFormatSingBox {
		if !oneOf(r.Format, "source", "binary") || r.Behavior != "" {
			return policyError(path+".format", "incompatible_format")
		}
	} else if !oneOf(r.Format, "yaml", "text", "mrs") || !oneOf(r.Behavior, "domain", "ipcidr", "classical") || (r.Format == "mrs" && r.Behavior == "classical") {
		return policyError(path+".format", "incompatible_format")
	}
	return nil
}
