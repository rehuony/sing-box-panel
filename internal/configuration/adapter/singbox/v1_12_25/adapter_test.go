// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11225

import (
	"encoding/json"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
)

func TestOfficialProfileMatchingIsExact(t *testing.T) {
	t.Parallel()

	for _, architecture := range []string{"amd64", "arm64"} {
		profile := officialProfile(t)
		profile.Architecture = architecture
		if !New().Supports(profile) {
			t.Fatalf("official %s profile was not supported", architecture)
		}
	}
	profile := officialProfile(t)
	tests := []struct {
		name   string
		mutate func(*adapter.Profile)
	}{
		{name: "nearby version", mutate: func(value *adapter.Profile) { value.ExactVersion = "1.12.24" }},
		{name: "other architecture", mutate: func(value *adapter.Profile) { value.Architecture = "riscv64" }},
		{name: "unreported features", mutate: func(value *adapter.Profile) { value.FeatureFingerprint = json.RawMessage(`{"status":"not_reported"}`) }},
		{name: "missing feature", mutate: func(value *adapter.Profile) { value.FeatureFingerprint = featureFingerprint(t, officialFeatures[1:]) }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := profile
			test.mutate(&candidate)
			if New().Supports(candidate) {
				t.Fatal("non-exact profile was supported")
			}
		})
	}
}

func TestProjectionRetainsSupportedSectionsAndStripsPanelMetadata(t *testing.T) {
	t.Parallel()

	canonicalJSON := []byte(`{"schema_version":2,"configuration":{"certificate":{"store":"system"},"services":[{"_panel":{"id":"service","enabled":true},"type":"resolved"}],"outbounds":[{"_panel":{"id":"disabled","enabled":false},"type":"direct","tag":"disabled"}]}}`)
	result, err := New().Project(adapter.Request{CanonicalJSON: canonicalJSON})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got, want := string(result.ConfigJSON), `{"certificate":{"store":"system"},"outbounds":[],"services":[{"type":"resolved"}]}`; got != want {
		t.Fatalf("config = %s, want %s", got, want)
	}
	if len(result.Diagnostics) != 0 || result.IgnoredDigest != "" {
		t.Fatalf("diagnostics = %+v, ignored digest = %q", result.Diagnostics, result.IgnoredDigest)
	}
}

func officialProfile(t *testing.T) adapter.Profile {
	t.Helper()
	return adapter.Profile{
		ExactVersion: Version, OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		FeatureFingerprint: featureFingerprint(t, officialFeatures),
	}
}

func featureFingerprint(t *testing.T, features []string) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(adapter.FeatureFingerprint{Status: "reported", Features: features})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
