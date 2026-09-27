// SPDX-License-Identifier: GPL-3.0-or-later
package configuration

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPresentationOrderPreservesValuesAndArrays(t *testing.T) {
	input := []byte(`{"future":{"z":false,"n":null,"large":900719925474099312345,"decimal":4.2000e+99},"route":{"rules":[{"outbound":"b","domain":["z","a"]},{"outbound":"a","domain":["a"]}]},"outbounds":[{"server_port":443,"tag":"b","type":"socks","server":"host"}],"log":{"level":"info"}}`)
	actual, err := FormatJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "log": {
    "level": "info"
  },
  "outbounds": [
    {
      "type": "socks",
      "tag": "b",
      "server": "host",
      "server_port": 443
    }
  ],
  "route": {
    "rules": [
      {
        "domain": [
          "z",
          "a"
        ],
        "outbound": "b"
      },
      {
        "domain": [
          "a"
        ],
        "outbound": "a"
      }
    ]
  },
  "future": {
    "decimal": 4.2000e+99,
    "large": 900719925474099312345,
    "n": null,
    "z": false
  }
}`
	if string(actual) != want {
		t.Fatalf("unexpected presentation:\n%s", actual)
	}
	again, err := FormatJSON(actual)
	if err != nil || !bytes.Equal(actual, again) {
		t.Fatal("formatting is not idempotent", err)
	}
	if _, err := FormatJSON([]byte(`{"log":`)); err == nil {
		t.Fatal("incomplete JSON accepted")
	}
}

func TestPresentationOrderGroupsProtocolAndTLSFields(t *testing.T) {
	input := []byte(`{"inbounds":[{"tls":{"key_path":"key.pem","certificate_path":"cert.pem","alpn":"h3","min_version":"1.2","server_name":"example.com","enabled":true},"users":[{"password":"test-b","name":"b"},{"password":"test-a","name":"a"}],"obfs":{"password":"test-obfs","type":"salamander"},"ignore_client_bandwidth":true,"tcp_fast_open":false,"listen_port":18053,"listen":"0.0.0.0","tag":"hy2-in","type":"hysteria2"},{"tls":{"key_path":"key.pem","certificate_path":"cert.pem","alpn":["h2","http/1.1"],"min_version":"1.2","server_name":"example.com","enabled":true},"users":[{"password":"test-anytls","name":"a"}],"tcp_fast_open":true,"reuse_addr":true,"listen_port":18443,"listen":"0.0.0.0","tag":"anytls-in","type":"anytls"}]}`)
	actual, err := FormatJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, actual); err != nil {
		t.Fatal(err)
	}
	want := `{"inbounds":[{"type":"hysteria2","tag":"hy2-in","listen":"0.0.0.0","listen_port":18053,"tcp_fast_open":false,"users":[{"name":"b","password":"test-b"},{"name":"a","password":"test-a"}],"ignore_client_bandwidth":true,"obfs":{"type":"salamander","password":"test-obfs"},"tls":{"enabled":true,"server_name":"example.com","alpn":"h3","min_version":"1.2","certificate_path":"cert.pem","key_path":"key.pem"}},{"type":"anytls","tag":"anytls-in","listen":"0.0.0.0","listen_port":18443,"reuse_addr":true,"tcp_fast_open":true,"users":[{"name":"a","password":"test-anytls"}],"tls":{"enabled":true,"server_name":"example.com","alpn":["h2","http/1.1"],"min_version":"1.2","certificate_path":"cert.pem","key_path":"key.pem"}}]}`
	if compact.String() != want {
		t.Fatalf("unexpected protocol and TLS presentation:\n%s", actual)
	}
}
