// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode"
)

// ChannelPolicy is owned by one channel. Node IDs are stable publication IDs,
// not names, protocols or the content digests used by legacy user grants.
type ChannelPolicy struct {
	Selection   NodeSelection   `json:"selection"`
	Organizer   NodeOrganizer   `json:"organizer"`
	Groups      []RuleGroup     `json:"groups"`
	DefaultExit RouteExit       `json:"default_exit"`
	Template    *NativeTemplate `json:"template,omitempty"`
}

type NodeSelection struct {
	IDs           []string `json:"ids"`
	ExcludedIDs   []string `json:"excluded_ids"`
	NewNodePolicy string   `json:"new_node_policy"`
}

type NodeOrganizer struct {
	Prefix       string   `json:"prefix"`
	ExcludeNames []string `json:"exclude_names"`
	Sort         string   `json:"sort"`
	Deduplicate  bool     `json:"deduplicate"`
	Incompatible string   `json:"incompatible"`
}

type RouteExit struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

type RuleGroup struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Enabled     bool          `json:"enabled"`
	NodeIDs     []string      `json:"node_ids"`
	DefaultExit RouteExit     `json:"default_exit"`
	Rules       []ChannelRule `json:"rules"`
}

// Rules and remote references share one ordered list. A disabled entry retains
// its metadata, but is not emitted. Nothing here fetches remote rule contents.
type ChannelRule struct {
	ID      string         `json:"id"`
	Enabled bool           `json:"enabled"`
	Kind    string         `json:"kind"`
	Value   string         `json:"value,omitempty"`
	Remote  *RemoteRuleSet `json:"remote,omitempty"`
	Exit    RouteExit      `json:"exit"`
}

type RemoteRuleSet struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Format         string `json:"format"`
	Behavior       string `json:"behavior,omitempty"`
	Accelerated    bool   `json:"accelerated"`
	UpdateInterval int    `json:"update_interval"`
}

type NativeTemplate struct {
	Format  RenderFormat `json:"format"`
	Content string       `json:"content"`
}

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

func ValidateChannelPolicy(p *ChannelPolicy, format RenderFormat) error {
	if p == nil {
		return nil
	}
	if format != RenderFormatSingBox && format != RenderFormatMihomo {
		return policyError("policy", "unsupported_client")
	}
	if !oneOf(p.Selection.NewNodePolicy, "include", "exclude") {
		return policyError("policy.selection.new_node_policy", "invalid_value")
	}
	if err := policyIDs(p.Selection.IDs, "policy.selection.ids"); err != nil {
		return err
	}
	if err := policyIDs(p.Selection.ExcludedIDs, "policy.selection.excluded_ids"); err != nil {
		return err
	}
	selected, excluded := stringSet(p.Selection.IDs), stringSet(p.Selection.ExcludedIDs)
	for id := range selected {
		if excluded[id] {
			return policyError("policy.selection", "conflicting_selection")
		}
	}
	if len(p.Organizer.Prefix) > 128 || strings.ContainsAny(p.Organizer.Prefix, "\x00\r\n,") || !oneOf(p.Organizer.Sort, "none", "name") || !oneOf(p.Organizer.Incompatible, "skip", "error") || len(p.Organizer.ExcludeNames) > 100 {
		return policyError("policy.organizer", "invalid_value")
	}
	for _, name := range p.Organizer.ExcludeNames {
		if name == "" || len(name) > 512 {
			return policyError("policy.organizer.exclude_names", "invalid_value")
		}
	}
	if len(p.Groups) > 256 {
		return policyError("policy.groups", "too_many_groups")
	}
	groups, names := map[string]bool{}, map[string]bool{}
	ruleCount := 0
	for i, group := range p.Groups {
		path := fmt.Sprintf("policy.groups[%d]", i)
		if !policyID(group.ID) || groups[group.ID] {
			return policyError(path+".id", "invalid_or_duplicate_id")
		}
		groups[group.ID] = true
		if !policyName(group.Name) || names[group.Name] || oneOf(group.Name, "direct", "DIRECT", "REJECT", "GLOBAL") {
			return policyError(path+".name", "invalid_or_duplicate_name")
		}
		names[group.Name] = true
		if err := policyIDs(group.NodeIDs, path+".node_ids"); err != nil {
			return err
		}
		candidates := stringSet(group.NodeIDs)
		for id := range candidates {
			if excluded[id] || (p.Selection.NewNodePolicy == "exclude" && !selected[id]) {
				return policyError(path+".node_ids", "node_not_selected")
			}
		}
		if err := validateExit(group.DefaultExit, candidates, nil, false, path+".default_exit"); err != nil {
			return err
		}
		rules := map[string]bool{}
		for j, rule := range group.Rules {
			ruleCount++
			if ruleCount > 5000 {
				return policyError("policy.groups", "too_many_rules")
			}
			rp := fmt.Sprintf("%s.rules[%d]", path, j)
			if !policyID(rule.ID) || rules[rule.ID] {
				return policyError(rp+".id", "invalid_or_duplicate_id")
			}
			rules[rule.ID] = true
			if err := validateExit(rule.Exit, candidates, nil, true, rp+".exit"); err != nil {
				return err
			}
			switch rule.Kind {
			case "domain", "domain_suffix", "domain_keyword":
				if !policyName(rule.Value) || strings.ContainsAny(rule.Value, " /:@") || rule.Remote != nil {
					return policyError(rp+".value", "invalid_domain")
				}
			case "ip_cidr":
				if _, err := netip.ParsePrefix(rule.Value); err != nil {
					if _, err = netip.ParseAddr(rule.Value); err != nil {
						return policyError(rp+".value", "invalid_ip_cidr")
					}
				}
				if rule.Remote != nil {
					return policyError(rp+".remote", "unexpected_field")
				}
			case "remote":
				if rule.Remote == nil || rule.Value != "" {
					return policyError(rp+".remote", "required")
				}
				if err := validateRemoteRuleSet(*rule.Remote, format, rp+".remote"); err != nil {
					return err
				}
			default:
				return policyError(rp+".kind", "unsupported_rule")
			}
		}
	}
	for _, g := range p.Groups {
		if !g.Enabled {
			delete(groups, g.ID)
		}
	}
	if err := validateExit(p.DefaultExit, selected, groups, false, "policy.default_exit"); err != nil {
		return err
	}
	_, err := parseChannelTemplate(p.Template, format)
	return err
}

func policyID(s string) bool {
	return s != "" && len(s) <= 128 && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == ':')
	}) < 0
}
func policyName(s string) bool {
	return s != "" && len(s) <= 128 && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0 && !strings.Contains(s, ",")
}
func stringSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}
func policyIDs(ids []string, path string) error {
	if len(ids) > MaximumNodes {
		return policyError(path, "too_many_nodes")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !policyID(id) || seen[id] {
			return policyError(path, "invalid_or_duplicate_id")
		}
		seen[id] = true
	}
	return nil
}
func validateExit(exit RouteExit, nodes, groups map[string]bool, inherit bool, path string) error {
	switch exit.Kind {
	case "direct", "reject":
		if exit.ID != "" {
			return policyError(path, "unexpected_id")
		}
	case "group-default":
		if !inherit || exit.ID != "" {
			return policyError(path, "invalid_inherited_exit")
		}
	case "node":
		if !nodes[exit.ID] {
			return policyError(path, "node_not_selected")
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
	if r.UpdateInterval < 60 || r.UpdateInterval > 2592000 {
		return policyError(path+".update_interval", "out_of_range")
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
