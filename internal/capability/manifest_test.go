// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestManifestDigestIsDeterministicAndDefensive(t *testing.T) {
	t.Parallel()

	firstEnum := make(map[string]string)
	firstEnum["direct"] = "direct"
	firstEnum["block"] = "reject"
	firstSpec := enumManifestSpec(t, firstEnum)
	first, err := NewManifest(firstSpec)
	if err != nil {
		t.Fatalf("NewManifest(first): %v", err)
	}
	firstDigest, err := first.Digest()
	if err != nil {
		t.Fatalf("first.Digest: %v", err)
	}

	// Mutating construction inputs after validation must not mutate Manifest.
	firstSpec.SemanticFacts[0].OwnedPaths[0] = "/tampered"
	firstSpec.Transforms[0].Enum["direct"] = "tampered"
	stableDigest, err := first.Digest()
	if err != nil {
		t.Fatalf("stable Digest: %v", err)
	}
	if stableDigest != firstDigest {
		t.Fatalf("manifest digest changed after caller mutated construction input: %s != %s", stableDigest, firstDigest)
	}

	secondEnum := make(map[string]string)
	secondEnum["block"] = "reject"
	secondEnum["direct"] = "direct"
	second, err := NewManifest(enumManifestSpec(t, secondEnum))
	if err != nil {
		t.Fatalf("NewManifest(second): %v", err)
	}
	secondDigest, err := second.Digest()
	if err != nil {
		t.Fatalf("second.Digest: %v", err)
	}
	if secondDigest != firstDigest {
		t.Fatalf("map insertion order changed digest: %s != %s", secondDigest, firstDigest)
	}
}

func TestManifestDigestNormalizesNumericConditions(t *testing.T) {
	t.Parallel()

	firstSpec := enumManifestSpec(t, map[string]string{"direct": "direct", "block": "reject"})
	firstSpec.UI[0].VisibleWhen = &VisibilityCondition{CanonicalPath: "/route/mode", Equals: json.Number("1.0")}
	secondSpec := enumManifestSpec(t, map[string]string{"direct": "direct", "block": "reject"})
	secondSpec.UI[0].VisibleWhen = &VisibilityCondition{CanonicalPath: "/route/mode", Equals: json.Number("1")}
	first, err := NewManifest(firstSpec)
	if err != nil {
		t.Fatalf("NewManifest(1.0): %v", err)
	}
	second, err := NewManifest(secondSpec)
	if err != nil {
		t.Fatalf("NewManifest(1): %v", err)
	}
	firstDigest, err := first.Digest()
	if err != nil {
		t.Fatalf("Digest(1.0): %v", err)
	}
	secondDigest, err := second.Digest()
	if err != nil {
		t.Fatalf("Digest(1): %v", err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("equivalent numeric conditions have different digests: %s != %s", firstDigest, secondDigest)
	}

	invalidSpec := enumManifestSpec(t, map[string]string{"direct": "direct", "block": "reject"})
	invalidSpec.UI[0].VisibleWhen = &VisibilityCondition{CanonicalPath: "/route/mode", Equals: json.Number("01")}
	if _, err := NewManifest(invalidSpec); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("NewManifest(invalid number) error = %v, want ErrInvalidManifest", err)
	}
}

func TestManifestRejectsOwnershipOverlap(t *testing.T) {
	t.Parallel()

	spec := ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{
			{ID: "dns", CanonicalPath: "/canonical/dns", Classification: CoverageSupported, OwnedPaths: []string{"/dns"}},
			{ID: "dns.server", CanonicalPath: "/canonical/server", Classification: CoverageSupported, OwnedPaths: []string{"/dns/server"}},
		},
	}
	if _, err := NewManifest(spec); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "ownership overlap") {
		t.Fatalf("NewManifest(overlap) error = %v, want ownership overlap", err)
	}
}

func TestManifestRejectsUnknownPrimitiveAndUnclassifiedFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ManifestSpec)
	}{
		{
			name: "unknown primitive",
			mutate: func(spec *ManifestSpec) {
				spec.Transforms[0].Primitive = "execute"
			},
		},
		{
			name: "missing classification",
			mutate: func(spec *ManifestSpec) {
				spec.SemanticFacts[0].Classification = ""
			},
		},
		{
			name: "transform references undeclared fact",
			mutate: func(spec *ManifestSpec) {
				spec.Transforms[0].FactID = "missing.fact"
			},
		},
		{
			name: "non-bijective enum",
			mutate: func(spec *ManifestSpec) {
				spec.Transforms[0].Enum["block"] = "direct"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := enumManifestSpec(t, map[string]string{"direct": "direct", "block": "reject"})
			test.mutate(&spec)
			if _, err := NewManifest(spec); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("NewManifest() error = %v, want ErrInvalidManifest", err)
			}
		})
	}
}

func TestDecodeManifestRejectsAmbiguousOrExecutableInput(t *testing.T) {
	t.Parallel()

	manifest, err := NewManifest(enumManifestSpec(t, map[string]string{"direct": "direct", "block": "reject"}))
	if err != nil {
		t.Fatalf("NewManifest: %v", err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	base := string(encoded)
	decoded, err := DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest(valid): %v", err)
	}
	originalDigest, err := manifest.Digest()
	if err != nil {
		t.Fatalf("manifest.Digest: %v", err)
	}
	decodedDigest, err := decoded.Digest()
	if err != nil {
		t.Fatalf("decoded.Digest: %v", err)
	}
	if decodedDigest != originalDigest {
		t.Fatalf("decoded manifest digest = %s, want %s", decodedDigest, originalDigest)
	}
	tests := []struct {
		name       string
		input      string
		wantDetail string
	}{
		{name: "duplicate", input: strings.Replace(base, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), wantDetail: "duplicate"},
		{name: "unknown", input: strings.TrimSuffix(base, "}") + `,"surprise":true}`, wantDetail: "unknown field"},
		{name: "remote ref", input: strings.TrimSuffix(base, "}") + `,"$ref":"https://example.invalid/code"}`, wantDetail: "forbidden"},
		{name: "script", input: strings.TrimSuffix(base, "}") + `,"script":"alert(1)"}`, wantDetail: "forbidden"},
		{name: "trailing", input: base + `{}`, wantDetail: "more than one"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeManifest([]byte(test.input))
			if !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), test.wantDetail) {
				t.Fatalf("DecodeManifest(%s) error = %v, want ErrInvalidManifest containing %q", test.name, err, test.wantDetail)
			}
		})
	}
}

func TestManifestResourceLimits(t *testing.T) {
	t.Parallel()

	tooManyFacts := make([]SemanticFact, MaximumSemanticFacts+1)
	if _, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: tooManyFacts,
	}); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("NewManifest(too many facts) error = %v, want resource limit", err)
	}

	oversized := make([]byte, MaximumManifestBytes+1)
	if _, err := DecodeManifest(oversized); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("DecodeManifest(oversized) error = %v, want resource limit", err)
	}

	deep := strings.Repeat("[", maximumJSONDepth+2) + "null" + strings.Repeat("]", maximumJSONDepth+2)
	if _, err := DecodeManifest([]byte(deep)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("DecodeManifest(deep) error = %v, want nesting limit", err)
	}
}

func TestReferenceValidationAndStrictJSON(t *testing.T) {
	t.Parallel()

	digest, err := coreartifact.ParseSHA256(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("ParseSHA256: %v", err)
	}
	reference, err := NewReference("SagerNet/sing-box", strings.Repeat("1", 40), digest)
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	encoded, err := json.Marshal(reference)
	if err != nil {
		t.Fatalf("Marshal reference: %v", err)
	}
	decoded, err := DecodeReference(encoded)
	if err != nil {
		t.Fatalf("DecodeReference: %v", err)
	}
	if decoded.Repository() != reference.Repository() || decoded.Commit() != reference.Commit() || decoded.Digest() != digest {
		t.Fatalf("decoded reference does not equal original")
	}

	invalid := []string{
		`{"repository":"owner/repo/extra","commit":"` + strings.Repeat("1", 40) + `","manifest_sha256":"` + digest.String() + `"}`,
		`{"repository":"owner/repo","commit":"` + strings.Repeat("0", 40) + `","manifest_sha256":"` + digest.String() + `"}`,
		strings.Replace(string(encoded), `"repository":`, `"unknown":true,"repository":`, 1),
		strings.Replace(string(encoded), `"commit":`, `"commit":"`+strings.Repeat("1", 40)+`","commit":`, 1),
	}
	for index, input := range invalid {
		if _, err := DecodeReference([]byte(input)); !errors.Is(err, ErrInvalidReference) {
			t.Fatalf("DecodeReference(invalid[%d]) error = %v, want ErrInvalidReference", index, err)
		}
	}
}

func TestManualManifestHasNoExecutableSurface(t *testing.T) {
	t.Parallel()

	manual, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.14.0"),
		SupportLevel:  SupportManualJSON,
	})
	if err != nil {
		t.Fatalf("NewManifest(manual): %v", err)
	}
	if _, err := NewProjector(manual); !errors.Is(err, ErrProjection) {
		t.Fatalf("NewProjector(manual) error = %v, want ErrProjection", err)
	}
}

func enumManifestSpec(t *testing.T, mapping map[string]string) ManifestSpec {
	t.Helper()
	return ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{{
			ID:             "route.mode",
			CanonicalPath:  "/route/mode",
			Classification: CoverageSupported,
			OwnedPaths:     []string{"/route_mode"},
		}},
		Transforms: []Transform{{
			ID:        "route.mode.enum",
			FactID:    "route.mode",
			Primitive: PrimitiveEnum,
			From:      []string{"/route/mode"},
			To:        []string{"/route_mode"},
			Enum:      mapping,
		}},
		UI: []UIDescriptor{{
			ID:     "route.mode.select",
			FactID: "route.mode",
			Kind:   UISelect,
			Label:  "Route mode",
			Options: []UIOption{
				{Value: "direct", Label: "Direct"},
				{Value: "block", Label: "Block"},
			},
		}},
	}
}

func version(t *testing.T, value string) coreartifact.ExactVersion {
	t.Helper()
	parsed, err := coreartifact.ParseExactVersion(value)
	if err != nil {
		t.Fatalf("ParseExactVersion(%q): %v", value, err)
	}
	return parsed
}
