// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11115

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
)

func TestOfficialProfileMatchingIsExact(t *testing.T) {
	t.Parallel()

	profile := officialProfile(t)
	if !New().Supports(profile) {
		t.Fatal("official profile was not supported")
	}
	tests := []struct {
		name   string
		mutate func(*adapter.Profile)
	}{
		{name: "nearby version", mutate: func(value *adapter.Profile) { value.ExactVersion = "1.11.16" }},
		{name: "other architecture", mutate: func(value *adapter.Profile) { value.Architecture = "amd64" }},
		{name: "unreported features", mutate: func(value *adapter.Profile) { value.FeatureFingerprint = json.RawMessage(`{"status":"not_reported"}`) }},
		{name: "extra feature", mutate: func(value *adapter.Profile) {
			features := append([]string(nil), officialFeatures...)
			features = append(features, "unexpected")
			value.FeatureFingerprint = featureFingerprint(t, features)
		}},
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

func TestProjectionIgnoresUnavailableFieldsWithoutChangingCanonicalIntent(t *testing.T) {
	t.Parallel()

	canonicalJSON := []byte(`{"schema_version":2,"configuration":{"log":{"level":"info"},"certificate":{"store":"system"},"services":[{"_panel":{"id":"service","enabled":true},"type":"resolved"}],"inbounds":[{"_panel":{"id":"disabled","enabled":false},"type":"mixed","tag":"disabled","listen_port":1080}]}}`)
	original := bytes.Clone(canonicalJSON)
	result, err := New().Project(adapter.Request{CanonicalJSON: canonicalJSON})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got, want := string(result.ConfigJSON), `{"inbounds":[],"log":{"level":"info"}}`; got != want {
		t.Fatalf("config = %s, want %s", got, want)
	}
	if len(result.Diagnostics) != 2 || result.IgnoredDigest == "" ||
		result.Diagnostics[0].Path != "/configuration/certificate" ||
		result.Diagnostics[1].Path != "/configuration/services" {
		t.Fatalf("diagnostics = %+v, ignored digest = %q", result.Diagnostics, result.IgnoredDigest)
	}
	document, err := canonical.ParseV2(canonicalJSON)
	if err != nil {
		t.Fatalf("ParseV2 after projection: %v", err)
	}
	configuration := document.Configuration()
	if _, found := configuration["certificate"]; !found {
		t.Fatal("certificate was removed from canonical intent")
	}
	if _, found := configuration["services"]; !found {
		t.Fatal("services were removed from canonical intent")
	}
	if !bytes.Equal(canonicalJSON, original) {
		t.Fatal("projection modified the caller-owned canonical bytes")
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
