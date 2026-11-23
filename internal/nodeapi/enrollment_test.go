// SPDX-License-Identifier: GPL-3.0-or-later

package nodeapi

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEnrollmentCodeRoundTripAndRejection(t *testing.T) {
	digest := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, code := range []EnrollmentCode{
		{Endpoint: "panel.example.com:3000", Pin: digest, Token: digest},
		{Endpoint: "www.example.com:443", Path: "/panel", Token: digest},
		{Endpoint: "[2001:db8::1]:443", Token: digest},
	} {
		parsed, err := ParseEnrollmentCode(code.String())
		if err != nil || parsed != code {
			t.Fatalf("round trip of %+v = %+v, %v", code, parsed, err)
		}
	}
	for _, raw := range []string{
		"",
		"sbp1.!",
		EnrollmentCode{Endpoint: "panel.example.com", Token: digest}.String(),
		EnrollmentCode{Endpoint: "panel.example.com:3001", Token: "short"}.String(),
		EnrollmentCode{Endpoint: "panel.example.com:3001", Pin: "short", Token: digest}.String(),
		EnrollmentCode{Endpoint: "panel.example.com:3001", Path: "/panel/", Token: digest}.String(),
		strings.TrimPrefix(EnrollmentCode{Endpoint: "panel.example.com:3001", Token: digest}.String(), "sbp1."),
	} {
		if _, err := ParseEnrollmentCode(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
