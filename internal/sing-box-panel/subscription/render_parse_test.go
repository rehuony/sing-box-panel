// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderRejectsStrictJSONViolationsWithSafeCodes(t *testing.T) {
	t.Parallel()

	secret := "do-not-reflect-this-secret"
	tests := []struct {
		name string
		data []byte
		code string
	}{
		{name: "empty", data: nil, code: "empty_document"},
		{name: "invalid utf8", data: []byte{'{', 0xff, '}'}, code: "invalid_utf8"},
		{name: "malformed", data: []byte(`{"outbounds":[`), code: "malformed_json"},
		{name: "duplicate nested key", data: []byte(`{"outbounds":[{"type":"shadowsocks","tag":"a","password":"` + secret + `","password":"again"}]}`), code: "duplicate_object_key"},
		{name: "trailing value", data: []byte(`{} {"password":"` + secret + `"}`), code: "trailing_json"},
		{name: "root array", data: []byte(`[]`), code: "root_not_object"},
		{name: "outbounds object", data: []byte(`{"outbounds":{}}`), code: "outbounds_not_array"},
		{name: "endpoints object", data: []byte(`{"endpoints":{}}`), code: "endpoints_not_array"},
		{name: "nul object key", data: []byte(`{"\u0000":"` + secret + `"}`), code: "invalid_object_key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Render(test.data, RenderChannel{Format: RenderFormatSingBox})
			if !errors.Is(err, ErrInvalidStartup) {
				t.Fatalf("Render error = %v, want ErrInvalidStartup", err)
			}
			var validation *RenderValidationError
			if !errors.As(err, &validation) || validation.Code() != test.code {
				t.Fatalf("Render error = %#v, want code %q", err, test.code)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error reflected secret: %v", err)
			}
		})
	}
}

func TestRenderRejectsSizeAndDepthBoundaries(t *testing.T) {
	t.Parallel()

	tooLarge := make([]byte, MaximumStartupBytes+1)
	for index := range tooLarge {
		tooLarge[index] = ' '
	}
	_, err := Render(tooLarge, RenderChannel{Format: RenderFormatSingBox})
	assertValidationCode(t, err, ErrInvalidStartup, "document_too_large")

	deep := `{"value":` + strings.Repeat("[", MaximumDocumentDepth+2) + `0` + strings.Repeat("]", MaximumDocumentDepth+2) + `}`
	_, err = Render([]byte(deep), RenderChannel{Format: RenderFormatSingBox})
	assertValidationCode(t, err, ErrInvalidStartup, "nesting_too_deep")
}

func TestRenderRejectsInvalidChannelWithoutParsingInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		channel RenderChannel
		code    string
	}{
		{name: "format", channel: RenderChannel{Format: "clash"}, code: "unsupported_format"},
		{name: "empty tag", channel: RenderChannel{Format: RenderFormatLoon, ExcludeTags: []string{""}}, code: "invalid_tag_exclusion"},
		{name: "duplicate tag", channel: RenderChannel{Format: RenderFormatLoon, ExcludeTags: []string{"a", "a"}}, code: "duplicate_tag_exclusion"},
		{name: "invalid type", channel: RenderChannel{Format: RenderFormatLoon, ExcludeTypes: []string{"VMess"}}, code: "invalid_type_exclusion"},
		{name: "duplicate type", channel: RenderChannel{Format: RenderFormatLoon, ExcludeTypes: []string{"vmess", "vmess"}}, code: "duplicate_type_exclusion"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Render([]byte(`not JSON and contains a secret`), test.channel)
			assertValidationCode(t, err, ErrInvalidChannel, test.code)
		})
	}
}

func TestRenderDiagnosesAllDuplicateOutboundTags(t *testing.T) {
	t.Parallel()

	result, err := Render([]byte(`{"outbounds":[
      {"type":"shadowsocks","tag":"duplicate","server":"a","server_port":1,"method":"none","password":"a"},
      {"type":"vmess","tag":"duplicate","server":"b","server_port":2,"uuid":"b"}
    ]}`), RenderChannel{Format: RenderFormatMihomo})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.NodeCount != 0 || len(result.Diagnostics) != 2 {
		t.Fatalf("result = %#v", result)
	}
	for index, diagnostic := range result.Diagnostics {
		if diagnostic.Collection != CollectionOutbounds || diagnostic.ItemIndex != index ||
			diagnostic.Code != DiagnosticDuplicateTag {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func assertValidationCode(t *testing.T, err error, scope error, code string) {
	t.Helper()
	if !errors.Is(err, scope) {
		t.Fatalf("error = %v, want %v", err, scope)
	}
	var validation *RenderValidationError
	if !errors.As(err, &validation) || validation.Code() != code {
		t.Fatalf("error = %#v, want validation code %q", err, code)
	}
}
