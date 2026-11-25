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
		{URL: "https://panel.example.com", Token: digest},
		{URL: "https://www.example.com:8443/panel", Token: digest},
		{URL: "http://127.0.0.1:3000", Token: digest},
		{URL: "http://[::1]:3000/panel", Token: digest},
	} {
		parsed, err := ParseEnrollmentCode(code.String())
		if err != nil || parsed != code {
			t.Fatalf("round trip of %+v = %+v, %v", code, parsed, err)
		}
	}
	for _, raw := range []string{
		"",
		"sbp2.!",
		// Node traffic never crosses a network unencrypted.
		EnrollmentCode{URL: "http://panel.example.com", Token: digest}.String(),
		EnrollmentCode{URL: "http://192.0.2.1:3000", Token: digest}.String(),
		EnrollmentCode{URL: "ws://127.0.0.1:3000", Token: digest}.String(),
		EnrollmentCode{URL: "https://panel.example.com/panel/", Token: digest}.String(),
		EnrollmentCode{URL: "https://user@panel.example.com", Token: digest}.String(),
		EnrollmentCode{URL: "https://panel.example.com", Token: "short"}.String(),
		strings.TrimPrefix(EnrollmentCode{URL: "https://panel.example.com", Token: digest}.String(), "sbp2."),
	} {
		if _, err := ParseEnrollmentCode(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
