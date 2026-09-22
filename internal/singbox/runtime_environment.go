// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

// RuntimeCompatibilityEnvironment preserves the reviewed 1.13 legacy DNS forms
// without rewriting the user's configuration. Later and unreviewed cores must
// use their own defaults; these upstream switches were removed in 1.14.
func RuntimeCompatibilityEnvironment(exactVersion string) []string {
	version, found := Lookup(exactVersion)
	if !found || version.SchemaSource != SchemaSourceReviewed113 {
		return nil
	}
	return []string{
		"ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true",
		"ENABLE_DEPRECATED_LEGACY_DNS_FAKEIP_OPTIONS=true",
		"ENABLE_DEPRECATED_OUTBOUND_DNS_RULE_ITEM=true",
		"ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true",
		"ENABLE_DEPRECATED_LEGACY_DOMAIN_STRATEGY_OPTIONS=true",
	}
}
