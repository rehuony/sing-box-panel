// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestChannelLoonRenderingAndMissingExits(t *testing.T) {
	nodes, policy := policyFixture(t, RenderFormatLoon)
	base := "# retained comment\n[General]\ndns-server = system,223.5.5.5\n[Host]\nexample.test = 127.0.0.1\n[MITM]\nenable = false\n"
	policy.Template = &NativeTemplate{Format: RenderFormatLoon, Content: base}
	for _, kind := range []string{"select", "url-test", "fallback"} {
		t.Run(kind, func(t *testing.T) {
			policy.Groups[0].Type = kind
			result, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatLoon}, policy)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{base, "[Proxy]\n", "Selected = " + kind + ",Tokyo,Hong Kong", "DOMAIN-SUFFIX,example.org,Selected", "IP-CIDR6,2001:db8::/32,DIRECT", "https://gh-proxy.com/https://raw.githubusercontent.com/example/rules/main/proxy.rules,policy=Hong Kong,enabled=true", "FINAL,Selected\n"} {
				if !bytes.Contains(result.Content, []byte(want)) {
					t.Fatalf("missing %q in %s", want, result.Content)
				}
			}
			again, err := RenderPolicyNodes(nodes, RenderChannel{Format: RenderFormatLoon}, policy)
			if err != nil || !bytes.Equal(result.Content, again.Content) || result.NodeCount != 2 || result.MediaType != "text/plain; charset=utf-8" {
				t.Fatal("unstable native output", err)
			}
			result, err = RenderPolicyNodes(nodes[1:], RenderChannel{Format: RenderFormatLoon}, policy)
			if err != nil || bytes.Contains(result.Content, []byte("tokyo.example.com")) {
				t.Fatal("unavailable node leaked", err)
			}
			if kind == "select" && (!bytes.Contains(result.Content, []byte("Selected = select,Hong Kong")) || !bytes.Contains(result.Content, []byte("FINAL,REJECT"))) {
				t.Fatal("missing first candidate did not reject", string(result.Content))
			}
			result, err = RenderPolicyNodes(nil, RenderChannel{Format: RenderFormatLoon}, policy)
			if err != nil || bytes.Contains(result.Content, []byte("[Proxy Group]")) || !bytes.Contains(result.Content, []byte("FINAL,REJECT\n")) || !bytes.Contains(result.Content, []byte("policy=REJECT,enabled=true")) {
				t.Fatal("empty group did not reject", err, string(result.Content))
			}
		})
	}
}

func TestChannelLoonRejectsConflictsAndInjection(t *testing.T) {
	for _, content := range []string{
		"{}", "key = value", "[General]\nbroken", "[General]\nx=1\nx=2", "[General]\n[General]",
		"[Proxy]\nsecret=password", "[Proxy Group]", "[Rule]", "[Remote Rule]", "[Remote Proxy]", "[Plugin]", "[Script]",
		"[General]\nx=1\r[Rule]\nFINAL,DIRECT", "[General]\nx=\x00",
	} {
		t.Run(content, func(t *testing.T) {
			_, err := parseChannelTemplate(&NativeTemplate{Format: RenderFormatLoon, Content: content}, RenderFormatLoon)
			var issue *PolicyError
			if !errors.As(err, &issue) || strings.Contains(err.Error(), "password") {
				t.Fatal("invalid template accepted or reflected", err)
			}
		})
	}
	for _, edit := range []func(*ChannelPolicy){
		func(p *ChannelPolicy) { p.Groups[0].Name = "PROXY" },
		func(p *ChannelPolicy) { p.Groups[0].Name = "option=value" },
		func(p *ChannelPolicy) { p.Groups[0].Name = "\"quoted\"" },
		func(p *ChannelPolicy) { p.Groups[0].Type = "url-test"; p.Groups[0].BuiltinNodes = []string{"direct"} },
		func(p *ChannelPolicy) { p.Groups[0].Rules[3].Remote.Format = "text" },
		func(p *ChannelPolicy) { p.Groups[0].Rules[3].Remote.UpdateInterval = 3600 },
		func(p *ChannelPolicy) { p.Groups[0].Rules[3].Remote.URL = "https://example.com/rules,policy=DIRECT" },
	} {
		_, p := policyFixture(t, RenderFormatLoon)
		edit(p)
		if err := ValidateChannelPolicy(p, RenderFormatLoon); err == nil {
			t.Fatal("incompatible Loon policy accepted")
		}
	}
}
