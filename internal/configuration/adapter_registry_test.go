// SPDX-License-Identifier: GPL-3.0-or-later

package configuration

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type testAdapter struct {
	version string
}

func (current testAdapter) ID() string           { return "test/" + current.version }
func (current testAdapter) Revision() string     { return "1" }
func (current testAdapter) ExactVersion() string { return current.version }
func (current testAdapter) Provenance() AdapterProvenance {
	return AdapterProvenance{UpstreamTag: "v" + current.version, UpstreamCommit: "commit", Source: "test"}
}
func (current testAdapter) Supports(profile CoreProfile) bool {
	return profile.ExactVersion == current.version
}
func (current testAdapter) Project(ProjectionRequest) (ProjectionResult, error) {
	return FinalizeProjection([]byte(`{}`), nil)
}

func TestRegistryUsesOnlyExactValidatedProfiles(t *testing.T) {
	t.Parallel()

	registry := MustNewAdapterRegistry(testAdapter{version: "1.13.19"}, testAdapter{version: "1.11.15"})
	if got, want := registry.Versions(), []string{"1.11.15", "1.13.19"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Versions = %v, want %v", got, want)
	}
	profile := CoreProfile{
		ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`),
	}
	resolved, err := registry.Resolve(profile)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.ExactVersion() != "1.13.19" {
		t.Fatalf("resolved version = %q", resolved.ExactVersion())
	}
	profile.ExactVersion = "1.13.18"
	if _, err := registry.Resolve(profile); !errors.Is(err, ErrUnsupportedCoreProfile) {
		t.Fatalf("Resolve nearby version error = %v, want ErrUnsupportedCoreProfile", err)
	}
	profile.ExactVersion = "v1.13.19"
	if _, err := registry.Resolve(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("Resolve malformed version error = %v, want ErrInvalidProfile", err)
	}
}

func TestRegistryRejectsInvalidAndDuplicateAdapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		adapters []Adapter
	}{
		{name: "nil", adapters: []Adapter{nil}},
		{name: "invalid version", adapters: []Adapter{testAdapter{version: "1.13"}}},
		{name: "duplicate", adapters: []Adapter{testAdapter{version: "1.13.19"}, testAdapter{version: "1.13.19"}}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Fatal("MustNewAdapterRegistry did not panic")
				}
			}()
			MustNewAdapterRegistry(test.adapters...)
		})
	}
}

func TestIgnoredDigestRequiresExactAcceptance(t *testing.T) {
	t.Parallel()

	result, err := FinalizeProjection([]byte(`{"log":{}}`), []ProjectionDiagnostic{{
		Class: DiagnosticIgnored, Code: "unsupported_field", Path: "/configuration/services",
		Message: "services are not supported",
	}})
	if err != nil {
		t.Fatalf("FinalizeProjection: %v", err)
	}
	if len(result.IgnoredDigest) != 64 {
		t.Fatalf("ignored digest = %q", result.IgnoredDigest)
	}
	if err := RequireIgnoredAcceptance(result, result.IgnoredDigest); err != nil {
		t.Fatalf("RequireIgnoredAcceptance exact digest: %v", err)
	}
	if err := RequireIgnoredAcceptance(result, ""); !errors.Is(err, ErrIgnoredNotAccepted) {
		t.Fatalf("RequireIgnoredAcceptance missing error = %v", err)
	}
}
