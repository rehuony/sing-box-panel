// SPDX-License-Identifier: GPL-3.0-or-later
package webauth

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	tests := map[string]string{
		"HTTPS://Example.COM:443": "https://example.com",
		"http://EXAMPLE.com:80":   "http://example.com",
		"https://[2001:db8::1]":   "https://[2001:db8::1]",
		"http://localhost:3000":   "http://localhost:3000",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := NormalizeOrigin(input)
			if err != nil || got != want {
				t.Fatalf("NormalizeOrigin(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}
	for _, input := range []string{"", "ftp://example.com", "https://user@example.com", "https://example.com/path", "https://example.com?x=1", "null"} {
		t.Run("reject_"+input, func(t *testing.T) {
			if _, err := NormalizeOrigin(input); err == nil {
				t.Fatalf("NormalizeOrigin(%q) unexpectedly succeeded", input)
			}
		})
	}
}
