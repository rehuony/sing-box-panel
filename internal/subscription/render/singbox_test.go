// SPDX-License-Identifier: GPL-3.0-or-later

package render

import (
	"bytes"
	"reflect"
	"testing"
)

func TestRenderSingBoxSortsFiltersAndClosesDependencies(t *testing.T) {
	t.Parallel()
	startup := []byte(`{"outbounds":[{"type":"direct","tag":"direct"},{"type":"shadowsocks","tag":"zulu","server":"z.example","server_port":443,"method":"aes-128-gcm","password":"z-secret"},{"type":"vless","tag":"alpha","server":"a.example","server_port":8443,"uuid":"uuid-a"},{"type":"shadowsocks","tag":"chain","server":"c.example","server_port":9443,"method":"aes-256-gcm","password":"c-secret","detour":"alpha"},{"type":"shadowsocks","tag":"orphan","server":"o.example","server_port":7443,"method":"aes-128-gcm","password":"o-secret","detour":"missing"},{"type":"shadowsocks","tag":"same","server":"one.example","server_port":1,"method":"none","password":"one"},{"type":"shadowsocks","tag":"same","server":"two.example","server_port":2,"method":"none","password":"two"},42]}`)
	result, err := Render(startup, Channel{Format: FormatSingBox})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `{"outbounds":[{"server":"a.example","server_port":8443,"tag":"alpha","type":"vless","uuid":"uuid-a"},{"detour":"alpha","method":"aes-256-gcm","password":"c-secret","server":"c.example","server_port":9443,"tag":"chain","type":"shadowsocks"},{"method":"aes-128-gcm","password":"z-secret","server":"z.example","server_port":443,"tag":"zulu","type":"shadowsocks"}]}` + "\n"
	if string(result.Content) != want || result.NodeCount != 3 || result.MediaType != "application/json" {
		t.Fatalf("result = %#v, content %s", result, result.Content)
	}
	wantDiagnostics := []Diagnostic{
		diagnostic(FormatSingBox, CollectionOutbounds, 4, DiagnosticUnresolvedDependency),
		diagnostic(FormatSingBox, CollectionOutbounds, 5, DiagnosticDuplicateTag),
		diagnostic(FormatSingBox, CollectionOutbounds, 6, DiagnosticDuplicateTag),
		diagnostic(FormatSingBox, CollectionOutbounds, 7, DiagnosticInvalidOutbound),
	}
	if !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("Diagnostics = %#v, want %#v", result.Diagnostics, wantDiagnostics)
	}
	filtered, err := Render(startup, Channel{Format: FormatSingBox, ExcludeTags: []string{"alpha"}})
	if err != nil {
		t.Fatalf("Render filtered: %v", err)
	}
	if filtered.NodeCount != 1 || !bytes.Contains(filtered.Content, []byte(`"tag":"zulu"`)) || bytes.Contains(filtered.Content, []byte(`"tag":"chain"`)) ||
		!containsDiagnostic(filtered.Diagnostics, CollectionOutbounds, 3, DiagnosticUnresolvedDependency) {
		t.Fatalf("filtered result = %#v, content %s", filtered, filtered.Content)
	}
}
