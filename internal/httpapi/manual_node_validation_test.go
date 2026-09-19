// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func TestManualNodeValidationPreservesExtensionsAndRejectsInvalidAssociations(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	for _, raw := range []string{
		`{"type":"socks","tag":"invalid","server":"proxy.example","server_port":1080,"udp_fragment":"wrong-type"}`,
		`{"type":"hysteria2","tag":"invalid","server":"proxy.example","server_port":443,"server_ports":["443:445"],"password":"secret-input","tls":{"enabled":true}}`,
		`{"type":"hysteria2","tag":"invalid","server":"proxy.example","server_port":443,"password":"secret-input","tls":{"enabled":false}}`,
		`{"type":"tuic","tag":"invalid","server":"proxy.example","server_port":443,"uuid":"5afd7249-7954-44e2-99a1-2c8497bdab1a","password":"secret-input","tls":{"enabled":true,"utls":{"enabled":true,"fingerprint":"chrome"}}}`,
		`{"type":"shadowsocks","tag":"invalid","server":"proxy.example","server_port":443,"method":"aes-128-gcm","password":"secret-input","udp_over_tcp":true,"multiplex":{"enabled":true}}`,
		`{"type":"socks","tag":"self","server":"proxy.example","server_port":1080,"detour":"self"}`,
		`{"type":"socks","tag":"unresolved","server":"proxy.example","server_port":1080,"detour":"missing"}`,
	} {
		response := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/nodes", `{"outbound":`+raw+`}`, "")
		if response.Code != 422 || strings.Contains(response.Body.String(), "secret-input") {
			t.Fatalf("invalid association: %d %s", response.Code, response.Body.String())
		}
	}
	for _, raw := range []string{
		`{"type":"socks","tag":"extension","server":"proxy.example","server_port":1080,"future":{"large":9007199254740993},"udp_fragment":false}`,
		`{"type":"hysteria2","tag":"hopping","server":"proxy.example","server_ports":["443:445"],"password":"secret-input","tls":{"enabled":true}}`,
		`{"type":"ssh","tag":"ssh-default-port","server":"proxy.example","user":"review","private_key_path":"client-key.pem"}`,
	} {
		response := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/nodes", `{"outbound":`+raw+`}`, "")
		if response.Code != 201 {
			t.Fatalf("valid node: %d %s", response.Code, response.Body.String())
		}
		var value application.SubscriptionNodeDetail
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if value.Name == "hopping" && (len(value.ServerPorts) != 1 || value.ServerPorts[0] != "443:445") {
			t.Fatal("port range missing in summary")
		}
		if value.Name == "ssh-default-port" && value.Port != 22 {
			t.Fatal("SSH default port missing")
		}
	}
}

func TestManualNodeCycleRejectedWithoutLosingPreviousConfiguration(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	create := func(raw string) application.SubscriptionNodeDetail {
		t.Helper()
		response := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/nodes", `{"outbound":`+raw+`}`, "")
		if response.Code != 201 {
			t.Fatalf("create: %d %s", response.Code, response.Body.String())
		}
		var value application.SubscriptionNodeDetail
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	a := create(`{"type":"socks","tag":"a","server":"a.example","server_port":1080}`)
	create(`{"type":"socks","tag":"b","server":"b.example","server_port":1080,"detour":"a"}`)
	endpoint := "/api/v1/subscription/nodes/" + a.ID
	response := authenticatedRequest(handler, http.MethodPut, endpoint, `{"revision":1,"outbound":{"type":"socks","tag":"a","server":"a.example","server_port":1080,"detour":"b"}}`, "")
	if response.Code != 422 || !strings.Contains(response.Body.String(), "dependency_cycle") {
		t.Fatalf("cycle: %d %s", response.Code, response.Body.String())
	}
	response = authenticatedRequest(handler, http.MethodGet, endpoint, "", "")
	var after application.SubscriptionNodeDetail
	if err := json.Unmarshal(response.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Revision != 1 || after.OutboundJSON != a.OutboundJSON {
		t.Fatal("rejected edit modified stored node")
	}
}
