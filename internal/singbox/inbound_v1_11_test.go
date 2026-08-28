// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"fmt"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func TestInbound11115ExactProtocolContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typeID string
		extra  string
		want   subscription.DiagnosticCode
	}{
		{typeID: "mixed"},
		{typeID: "vmess", extra: `,"users":[{"name":"one","uuid":"uuid"}]`},
		{typeID: "hysteria2", extra: `,"users":[{"name":"one","password":"pass"}],"tls":{"enabled":true}`},
		{typeID: "anytls", extra: `,"users":[{"name":"one","password":"pass"}]`, want: subscription.DiagnosticUnsupportedType},
		{typeID: "naive", extra: `,"users":[{"username":"one","password":"pass"}]`, want: subscription.DiagnosticUnsupportedType},
		{typeID: "snell", extra: `,"psk":"pass"`, want: subscription.DiagnosticUnsupportedType},
	}
	for _, test := range tests {
		test := test
		t.Run(test.typeID, func(t *testing.T) {
			t.Parallel()
			document := []byte(fmt.Sprintf(`{"inbounds":[{"type":%q,"tag":%q,"listen_port":443%s}]}`, test.typeID, test.typeID, test.extra))
			result, err := NewInboundRegistry().Convert(
				"1.11.15",
				subscription.InboundRequest{FinalStartupJSON: document, PublicHost: "public.example"},
			)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if test.want == "" {
				if len(result.Nodes) != 1 || len(result.Diagnostics) != 0 {
					t.Fatalf("nodes=%+v diagnostics=%+v", result.Nodes, result.Diagnostics)
				}
				return
			}
			if len(result.Nodes) != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != test.want {
				t.Fatalf("nodes=%+v diagnostics=%+v", result.Nodes, result.Diagnostics)
			}
		})
	}
}
