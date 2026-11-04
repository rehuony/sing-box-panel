// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

const (
	maximumChannelConfigBytes = 640 << 10
	maximumChannelExclusions  = 10_000
)

// ChannelConfig is the strict, renderer-owned channel policy.
type ChannelConfig struct {
	Policy       *ChannelPolicy `json:"policy,omitempty"`
	ExcludeTags  []string       `json:"exclude_tags,omitempty"`
	ExcludeTypes []string       `json:"exclude_types,omitempty"`
}

// DecodeChannelConfig validates and returns the normalized channel config.
func DecodeChannelConfig(raw json.RawMessage) (ChannelConfig, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ChannelConfig{}, errors.New("subscription channel config must be a non-null JSON object")
	}
	var config ChannelConfig
	if err := jsonstrict.Decode(raw, maximumChannelConfigBytes, &config); err != nil {
		return ChannelConfig{}, fmt.Errorf("subscription channel config: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := jsonstrict.Decode(raw, maximumChannelConfigBytes, &fields); err != nil || fields == nil {
		return ChannelConfig{}, errors.New("subscription channel config must be a non-null JSON object")
	}
	for name, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ChannelConfig{}, fmt.Errorf("subscription channel config field %q cannot be null", name)
		}
	}
	if config.ExcludeTags == nil {
		config.ExcludeTags = []string{}
	}
	if config.ExcludeTypes == nil {
		config.ExcludeTypes = []string{}
	}
	if len(config.ExcludeTags) > maximumChannelExclusions || len(config.ExcludeTypes) > maximumChannelExclusions {
		return ChannelConfig{}, errors.New("subscription channel config has too many exclusions")
	}
	if err := validateUniqueExclusions(config.ExcludeTags, validExclusionTag, "tag"); err != nil {
		return ChannelConfig{}, err
	}
	if err := validateUniqueExclusions(config.ExcludeTypes, ValidType, "type"); err != nil {
		return ChannelConfig{}, err
	}
	if p := config.Policy; p != nil {
		if len(config.ExcludeTags) > 0 || len(config.ExcludeTypes) > 0 {
			return ChannelConfig{}, &PolicyError{Path: "policy.selection", Code: "legacy_filters_not_supported"}
		}
		p.Selection.IDs = append([]string{}, p.Selection.IDs...)
		p.Selection.ExcludedIDs = append([]string{}, p.Selection.ExcludedIDs...)
		p.Groups = append([]RuleGroup{}, p.Groups...)
		for i := range p.Groups {
			p.Groups[i].NodeIDs = append([]string{}, p.Groups[i].NodeIDs...)
			p.Groups[i].Rules = append([]ChannelRule{}, p.Groups[i].Rules...)
		}
	}
	return config, nil
}

// CanonicalChannelConfig validates a channel config for one output format
// and returns the canonical bytes stored with the channel.
func CanonicalChannelConfig(raw json.RawMessage, format RenderFormat) (json.RawMessage, error) {
	config, err := DecodeChannelConfig(raw)
	if err != nil {
		return nil, err
	}
	if err := ValidateChannelPolicy(config.Policy, format); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode subscription channel config: %w", err)
	}
	if len(encoded) > maximumChannelConfigBytes {
		return nil, errors.New("subscription channel config exceeds size limit")
	}
	return encoded, nil
}

func validateUniqueExclusions(
	values []string,
	valid func(string) bool,
	kind string,
) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !valid(value) {
			return fmt.Errorf("subscription channel config contains an invalid %s exclusion", kind)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("subscription channel config contains a duplicate %s exclusion", kind)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validExclusionTag(value string) bool {
	return value != "" && len(value) <= 512 && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
