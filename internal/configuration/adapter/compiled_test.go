// SPDX-License-Identifier: GPL-3.0-or-later

package adapter_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
	singbox11115 "github.com/rehuony/sing-box-panel/internal/configuration/adapter/singbox/v1_11_15"
	singbox11225 "github.com/rehuony/sing-box-panel/internal/configuration/adapter/singbox/v1_12_25"
	singbox11319 "github.com/rehuony/sing-box-panel/internal/configuration/adapter/singbox/v1_13_19"
)

func TestCompiledRegistryDispatchesExactOfficialProfiles(t *testing.T) {
	t.Parallel()

	registry := adapter.MustNewRegistry(singbox11115.New(), singbox11225.New(), singbox11319.New())
	if got, want := registry.Versions(), []string{"1.11.15", "1.12.25", "1.13.19"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Versions = %v, want %v", got, want)
	}
	tests := []struct {
		version  string
		features []string
	}{
		{version: "1.11.15", features: []string{"with_acme", "with_clash_api", "with_dhcp", "with_ech", "with_gvisor", "with_quic", "with_reality_server", "with_utls", "with_wireguard"}},
		{version: "1.12.25", features: []string{"with_acme", "with_clash_api", "with_dhcp", "with_gvisor", "with_quic", "with_tailscale", "with_utls", "with_wireguard"}},
		{version: "1.13.19", features: []string{"badlinkname", "tfogo_checklinkname0", "with_acme", "with_ccm", "with_clash_api", "with_dhcp", "with_gvisor", "with_ocm", "with_quic", "with_tailscale", "with_utls", "with_wireguard"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.version, func(t *testing.T) {
			t.Parallel()
			fingerprint, err := json.Marshal(adapter.FeatureFingerprint{Status: "reported", Features: test.features})
			if err != nil {
				t.Fatalf("Marshal fingerprint: %v", err)
			}
			profile := adapter.Profile{
				ExactVersion: test.version, OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
				FeatureFingerprint: fingerprint,
			}
			resolved, err := registry.Resolve(profile)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if resolved.ExactVersion() != test.version {
				t.Fatalf("resolved version = %q, want %q", resolved.ExactVersion(), test.version)
			}
			profile.Architecture = "amd64"
			if _, err := registry.Resolve(profile); !errors.Is(err, adapter.ErrUnsupportedCoreProfile) {
				t.Fatalf("Resolve amd64 error = %v, want ErrUnsupportedCoreProfile", err)
			}
			profile.Architecture = "arm64"
			profile.FeatureFingerprint = json.RawMessage(`{"status":"reported","features":["with_quic"]}`)
			if _, err := registry.Resolve(profile); !errors.Is(err, adapter.ErrUnsupportedCoreProfile) {
				t.Fatalf("Resolve altered fingerprint error = %v, want ErrUnsupportedCoreProfile", err)
			}
		})
	}
}

func TestVersionSwitchPreservesCanonicalIntentAndReportsIgnoredFields(t *testing.T) {
	t.Parallel()

	canonicalJSON := []byte(`{
		"schema_version":2,
		"configuration":{
			"certificate":{"store":"system"},
			"services":[{"_panel":{"id":"resolved","enabled":true},"type":"resolved","tag":"resolved"}],
			"inbounds":[{"_panel":{"id":"mixed","enabled":true},"type":"mixed","tag":"mixed","listen_port":1080}]
		}
	}`)
	newer, err := singbox11319.New().Project(adapter.Request{CanonicalJSON: canonicalJSON})
	if err != nil {
		t.Fatalf("Project 1.13.19: %v", err)
	}
	older, err := singbox11115.New().Project(adapter.Request{CanonicalJSON: canonicalJSON})
	if err != nil {
		t.Fatalf("Project 1.11.15: %v", err)
	}
	if older.IgnoredDigest == "" || len(older.Diagnostics) != 2 {
		t.Fatalf("1.11.15 ignored result = %+v", older)
	}
	if string(newer.ConfigJSON) != `{"certificate":{"store":"system"},"inbounds":[{"listen_port":1080,"tag":"mixed","type":"mixed"}],"services":[{"tag":"resolved","type":"resolved"}]}` {
		t.Fatalf("1.13.19 config = %s", newer.ConfigJSON)
	}
	if string(older.ConfigJSON) != `{"inbounds":[{"listen_port":1080,"tag":"mixed","type":"mixed"}]}` {
		t.Fatalf("1.11.15 config = %s", older.ConfigJSON)
	}
	newerAgain, err := singbox11319.New().Project(adapter.Request{CanonicalJSON: canonicalJSON})
	if err != nil {
		t.Fatalf("Project 1.13.19 again: %v", err)
	}
	if !reflect.DeepEqual(newer, newerAgain) {
		t.Fatalf("switching back changed projection: first=%+v again=%+v", newer, newerAgain)
	}
}
