// SPDX-License-Identifier: GPL-3.0-or-later

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticsNeverContainOutboundValues(t *testing.T) {
	t.Parallel()
	const secret = "credential-that-must-not-leak"
	startup := []byte(`{"outbounds":[{"type":"vmess","tag":"sensitive-tag","server":"sensitive.example","server_port":443,"uuid":"` + secret + `","transport":{"type":"grpc"}}]}`)
	result, err := Render(startup, Channel{Format: FormatLoon})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	encoded, err := json.Marshal(result.Diagnostics)
	if err != nil {
		t.Fatalf("Marshal diagnostics: %v", err)
	}
	for _, sensitive := range []string{secret, "sensitive-tag", "sensitive.example"} {
		if strings.Contains(string(encoded), sensitive) {
			t.Fatalf("diagnostics leaked %q: %s", sensitive, encoded)
		}
	}
}
