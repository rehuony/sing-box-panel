// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11319

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription/inbound"
	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

type inboundCredential struct {
	identity string
	label    string
	value    map[string]any
}

func inboundCredentials(typeID string, inboundValue map[string]any) ([]inboundCredential, error) {
	rawUsers, hasUsers := inboundValue["users"]
	if !hasUsers {
		return []inboundCredential{{identity: "default", label: "default", value: inboundValue}}, nil
	}
	users, ok := rawUsers.([]any)
	if !ok || len(users) == 0 || len(users) > node.MaximumNodes {
		return nil, inbound.ErrAmbiguousInboundCredential
	}
	result := make([]inboundCredential, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for index, rawUser := range users {
		user, ok := rawUser.(map[string]any)
		if !ok {
			return nil, inbound.ErrAmbiguousInboundCredential
		}
		identity := firstNonEmptyString(user, "name", "username", "uuid")
		label := identity
		if identity == "" && len(users) == 1 {
			identity = "default"
			label = "default"
		}
		if identity == "" {
			return nil, inbound.ErrAmbiguousInboundCredential
		}
		identityKey := strings.ToLower(typeID + "\x00" + identity)
		if _, duplicate := seen[identityKey]; duplicate {
			return nil, inbound.ErrAmbiguousInboundCredential
		}
		seen[identityKey] = struct{}{}
		if label == "" {
			label = fmt.Sprintf("user-%d", index+1)
		}
		result = append(result, inboundCredential{identity: identity, label: label, value: user})
	}
	return result, nil
}

func validateConvertedCredential(typeID string, outbound map[string]any) error {
	required := [][]string{}
	switch typeID {
	case "shadowsocks", "trojan", "shadowtls", "hysteria2", "anytls":
		required = [][]string{{"password"}}
	case "vmess", "vless":
		required = [][]string{{"uuid"}}
	case "naive":
		required = [][]string{{"username"}, {"password"}}
	case "hysteria":
		required = [][]string{{"auth", "auth_str"}}
	case "tuic":
		required = [][]string{{"uuid"}, {"password"}}
	}
	for _, alternatives := range required {
		present := false
		for _, key := range alternatives {
			if value, ok := outbound[key].(string); ok && value != "" {
				present = true
			}
		}
		if !present {
			return errors.New("missing required inbound credential field")
		}
	}
	return nil
}

func firstNonEmptyString(value map[string]any, names ...string) string {
	for _, name := range names {
		if text, ok := value[name].(string); ok && text != "" {
			return text
		}
	}
	return ""
}
