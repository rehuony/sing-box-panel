// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"
	"strings"
	"unicode"
)

// The base owns only these sections. Keep its original lines rather than
// decoding values and losing comments, quoting or client-specific options.
func parseLoonChannelTemplate(content string) (channelTemplate, error) {
	section := ""
	sections := map[string]bool{}
	keys := map[string]bool{}
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		path := fmt.Sprintf("policy.template.content@%d", i+1)
		if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\r' }) >= 0 || strings.Contains(strings.TrimSuffix(raw, "\r"), "\r") {
			return channelTemplate{}, policyError(path, "invalid_loon_structure")
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return channelTemplate{}, policyError(path, "invalid_loon_structure")
			}
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if oneOf(section, "proxy", "proxy group", "rule", "remote rule", "remote proxy", "remote filter") {
				return channelTemplate{}, policyError(path, "generated_field")
			}
			if !oneOf(section, "general", "host", "mitm") {
				return channelTemplate{}, policyError(path, "unsupported_section")
			}
			if sections[section] {
				return channelTemplate{}, policyError(path, "duplicate_section")
			}
			sections[section] = true
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if section == "" || !ok || key == "" {
			return channelTemplate{}, policyError(path, "invalid_loon_structure")
		}
		key = section + "/" + key
		if keys[key] {
			return channelTemplate{}, policyError(path, "duplicate_key")
		}
		keys[key] = true
	}
	return channelTemplate{loon: content}, nil
}

func validLoonPolicyName(value string) bool {
	return validLoonName(value) && validLoonAtom(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func renderLoonChannel(base string, nodes []loonNode, groups []map[string]any, rules []any, remote []string, final string) []byte {
	var output strings.Builder
	if base != "" {
		output.WriteString(base)
		if !strings.HasSuffix(base, "\n") {
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}
	output.Write(marshalLoon(nodes))
	if len(groups) > 0 {
		output.WriteString("\n[Proxy Group]\n")
		for _, group := range groups {
			fmt.Fprintf(&output, "%s = %s,%s", group["name"], group["type"], strings.Join(group["proxies"].([]string), ","))
			if group["type"] != "select" {
				fmt.Fprintf(&output, ",url=%s,interval=%d", group["url"], group["interval"])
				if group["type"] == "url-test" {
					fmt.Fprintf(&output, ",tolerance=%d", group["tolerance"])
				}
			}
			output.WriteByte('\n')
		}
	}
	if len(remote) > 0 {
		output.WriteString("\n[Remote Rule]\n")
		for _, line := range remote {
			output.WriteString(line + "\n")
		}
	}
	output.WriteString("\n[Rule]\n")
	for _, rule := range rules {
		output.WriteString(rule.(string) + "\n")
	}
	output.WriteString("FINAL," + final + "\n")
	return []byte(output.String())
}
