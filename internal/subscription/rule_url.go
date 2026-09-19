// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"net/url"
	"strings"
)

// Existing proxy hosts are recognized for unwrapping only. No network request
// or authorization header is involved in producing a client-owned reference.
var ruleProxyHosts = map[string]bool{"mirror.ghproxy.com": true, "gh.api.99988866.xyz": true, "gh-proxy.com": true, "ghproxy.com": true, "ghproxy.net": true, "ghfast.top": true, "hub.gitmirror.com": true}

func DirectRuleURL(raw string) string {
	value := strings.TrimSpace(raw)
	for range 16 {
		u, err := url.Parse(value)
		if err != nil || u.User != nil || !oneOf(u.Scheme, "http", "https") {
			break
		}
		host := strings.ToLower(u.Hostname())
		known := ruleProxyHosts[host] || strings.HasSuffix(host, ".ghproxy.com") || strings.HasSuffix(host, ".ghproxy.net")
		if !known {
			break
		}
		next := strings.TrimPrefix(u.EscapedPath(), "/")
		if u.RawQuery != "" {
			next += "?" + u.RawQuery
		}
		// Prefix wrappers carry the complete source URL verbatim; preserve percent
		// escapes rather than decoding the GitHub path or signed query.
		start := strings.Index(value, "://")
		slash := strings.Index(value[start+3:], "/")
		if slash >= 0 {
			next = value[start+3+slash+1:]
		}
		if !strings.HasPrefix(next, "https://") && !strings.HasPrefix(next, "http://") {
			break
		}
		value = next
	}
	return value
}
func CanAccelerateRuleURL(raw string) bool {
	u, err := url.Parse(DirectRuleURL(raw))
	if err != nil || u.User != nil || !oneOf(u.Scheme, "http", "https") || u.Port() != "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || strings.HasSuffix(h, ".github.com") || h == "raw.githubusercontent.com" || h == "gist.githubusercontent.com"
}
func EffectiveRuleURL(raw string, accelerated bool) string {
	direct := DirectRuleURL(raw)
	if accelerated && CanAccelerateRuleURL(direct) {
		return "https://gh-proxy.com/" + direct
	}
	return direct
}
