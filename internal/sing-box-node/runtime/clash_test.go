// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseEndpointRequiresLoopbackAndSecret(t *testing.T) {
	valid, err := ParseClashEndpoint([]byte(`{
        // final config is allowed to be JSONC
        "experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","secret":"secret"}}
    }`))
	if err != nil || valid.BaseURL != "http://127.0.0.1:9090" || valid.Secret != "secret" {
		t.Fatalf("ParseClashEndpoint(valid) = %+v, %v", valid, err)
	}
	for _, raw := range []string{
		`{}`,
		`{"experimental":{"clash_api":{"external_controller":"0.0.0.0:9090","secret":"x"}}}`,
		`{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","secret":""}}}`,
	} {
		if _, err := ParseClashEndpoint([]byte(raw)); !errors.Is(err, ErrInvalidClashConfig) {
			t.Fatalf("ParseClashEndpoint(%s) error = %v", raw, err)
		}
	}
}

func TestClientReadsVersionAndConnectionsWithBearerSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			http.Error(writer, "denied", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/version":
			_, _ = writer.Write([]byte(`{"version":"sing-box 1.13.19"}`))
		case "/connections":
			_, _ = writer.Write([]byte(`{"memory":42,"uploadTotal":12,"downloadTotal":34,"connections":[{},{}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	endpoint, err := ParseClashEndpoint([]byte(fmt.Sprintf(
		`{"experimental":{"clash_api":{"external_controller":%q,"secret":"secret"}}}`,
		strings.TrimPrefix(server.URL, "http://"),
	)))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClashClient(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	version, err := client.Version(context.Background())
	if err != nil || version != "1.13.19" {
		t.Fatalf("Version() = %q, %v", version, err)
	}
	sample, err := client.Connections(context.Background())
	if err != nil || sample.Memory != 42 || sample.Connections != 2 || sample.UploadTotal != 12 || sample.DownloadTotal != 34 {
		t.Fatalf("Connections() = %+v, %v", sample, err)
	}
}

func TestNormalizeVersionAcceptsOnlyExactStableOfficialForms(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"1.13.19", "v1.13.19", "sing-box 1.13.19", "sing-box v1.13.19"} {
		version, err := normalizeVersion(input)
		if err != nil || version != "1.13.19" {
			t.Fatalf("normalizeVersion(%q) = %q, %v", input, version, err)
		}
	}
	for _, input := range []string{"", "sing-box", "sing-box version 1.13.19", "wrapper 1.13.19", "1.14.0-beta.1"} {
		if _, err := normalizeVersion(input); err == nil {
			t.Fatalf("normalizeVersion(%q) succeeded", input)
		}
	}
}
