// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

// hasRequiredCredential reports whether outbound carries every credential
// field its protocol needs, for imported sources and converted inbounds alike.
func hasRequiredCredential(typeID string, outbound map[string]any) bool {
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
			return false
		}
	}
	return true
}
