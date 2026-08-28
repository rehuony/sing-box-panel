// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import "errors"

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
	case "snell":
		required = [][]string{{"psk"}}
	}
	for _, alternatives := range required {
		present := false
		for _, key := range alternatives {
			if value, ok := outbound[key].(string); ok && value != "" {
				present = true
			}
		}
		if !present {
			return errors.New("missing required source credential field")
		}
	}
	return nil
}
