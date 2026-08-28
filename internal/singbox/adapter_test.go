// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestCompiledRegistryDispatchesExactOfficialProfiles(t *testing.T) {
	t.Parallel()

	registry := NewConfigurationRegistry()
	supported := Versions()
	wantVersions := make([]string, len(supported))
	for index, version := range supported {
		wantVersions[index] = version.ExactVersion
	}
	if got := registry.Versions(); !reflect.DeepEqual(got, wantVersions) {
		t.Fatalf("Versions = %v, want %v", got, wantVersions)
	}
	for _, supportedVersion := range supported {
		supportedVersion := supportedVersion
		t.Run(supportedVersion.ExactVersion, func(t *testing.T) {
			t.Parallel()
			for _, architecture := range []string{"amd64", "arm64"} {
				fingerprint, err := json.Marshal(configuration.FeatureFingerprint{
					Status: "reported", Features: supportedVersion.Profiles[architecture].Features,
				})
				if err != nil {
					t.Fatalf("Marshal fingerprint: %v", err)
				}
				profile := configuration.CoreProfile{
					ExactVersion: supportedVersion.ExactVersion, OperatingSystem: "linux", Architecture: architecture, Variant: "plain",
					FeatureFingerprint: fingerprint,
				}
				resolved, err := registry.Resolve(profile)
				if err != nil {
					t.Fatalf("Resolve %s: %v", architecture, err)
				}
				if resolved.ExactVersion() != supportedVersion.ExactVersion {
					t.Fatalf("resolved version = %q, want %q", resolved.ExactVersion(), supportedVersion.ExactVersion)
				}
			}
			fingerprint, _ := json.Marshal(configuration.FeatureFingerprint{
				Status: "reported", Features: supportedVersion.Profiles["arm64"].Features,
			})
			profile := configuration.CoreProfile{
				ExactVersion: supportedVersion.ExactVersion, OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
				FeatureFingerprint: fingerprint,
			}
			profile.FeatureFingerprint = json.RawMessage(`{"status":"reported","features":["with_quic"]}`)
			if _, err := registry.Resolve(profile); !errors.Is(err, configuration.ErrUnsupportedCoreProfile) {
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
	registry := NewConfigurationRegistry()
	newer := projectVersion(t, registry, "1.13.19", canonicalJSON)
	older := projectVersion(t, registry, "1.11.15", canonicalJSON)
	if older.IgnoredDigest == "" || len(older.Diagnostics) != 2 {
		t.Fatalf("1.11.15 ignored result = %+v", older)
	}
	if string(newer.ConfigJSON) != `{"certificate":{"store":"system"},"inbounds":[{"listen_port":1080,"tag":"mixed","type":"mixed"}],"services":[{"tag":"resolved","type":"resolved"}]}` {
		t.Fatalf("1.13.19 config = %s", newer.ConfigJSON)
	}
	if string(older.ConfigJSON) != `{"inbounds":[{"listen_port":1080,"tag":"mixed","type":"mixed"}]}` {
		t.Fatalf("1.11.15 config = %s", older.ConfigJSON)
	}
	newerAgain := projectVersion(t, registry, "1.13.19", canonicalJSON)
	if !reflect.DeepEqual(newer, newerAgain) {
		t.Fatalf("switching back changed projection: first=%+v again=%+v", newer, newerAgain)
	}
}

func projectVersion(
	t *testing.T,
	registry *configuration.AdapterRegistry,
	exactVersion string,
	canonicalJSON []byte,
) configuration.ProjectionResult {
	t.Helper()
	version, ok := Lookup(exactVersion)
	if !ok {
		t.Fatalf("lookup %s", exactVersion)
	}
	fingerprint, err := json.Marshal(configuration.FeatureFingerprint{
		Status: "reported", Features: version.Profiles[ArchitectureARM64].Features,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Project(configuration.CoreProfile{
		ExactVersion: exactVersion, OperatingSystem: "linux", Architecture: ArchitectureARM64, Variant: "plain",
		FeatureFingerprint: fingerprint,
	}, configuration.ProjectionRequest{CanonicalJSON: canonicalJSON})
	if err != nil {
		t.Fatalf("Project %s: %v", exactVersion, err)
	}
	return result
}
