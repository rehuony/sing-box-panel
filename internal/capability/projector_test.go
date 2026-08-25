// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"reflect"
	"testing"
)

func TestProjectorRoundTripAllPrimitives(t *testing.T) {
	t.Parallel()

	manifest := allPrimitivesManifest(t)
	projector, err := NewProjector(manifest)
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}
	canonical := map[string]any{
		"canonical": map[string]any{
			"name":      "alpha",
			"wrapped":   "inside",
			"unwrapped": map[string]any{"value": "outside"},
			"split":     "example.com:443",
			"join":      map[string]any{"host": "127.0.0.1", "port": "1080"},
			"mode":      "block",
			"enabled":   true,
			"conditional": map[string]any{
				"value": "only-when-enabled",
			},
			"nullable": nil,
		},
	}

	projection, err := projector.Project(canonical)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(projection.Diagnostics) != 0 {
		t.Fatalf("Project diagnostics = %+v, want none", projection.Diagnostics)
	}
	wantVersion := map[string]any{
		"legacy": map[string]any{
			"name":      "alpha",
			"wrapped":   map[string]any{"value": "inside"},
			"unwrapped": "outside",
			"split":     map[string]any{"host": "example.com", "port": "443"},
			"join":      "127.0.0.1:1080",
			"mode":      "reject",
			"enabled":   true,
			"conditional": map[string]any{
				"value": "only-when-enabled",
			},
			"nullable": nil,
		},
	}
	if !reflect.DeepEqual(projection.Document, wantVersion) {
		t.Fatalf("Project document = %#v, want %#v", projection.Document, wantVersion)
	}

	reversed, err := projector.Reverse(projection.Document)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if !reflect.DeepEqual(reversed, canonical) {
		t.Fatalf("round trip = %#v, want %#v", reversed, canonical)
	}
}

func TestProjectorRenameRoundTripJSONValues(t *testing.T) {
	t.Parallel()

	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{{
			ID:             "value",
			CanonicalPath:  "/value",
			Classification: CoverageSupported,
			OwnedPaths:     []string{"/renamed"},
		}},
		Transforms: []Transform{{
			ID: "value.rename", FactID: "value", Primitive: PrimitiveRename,
			From: []string{"/value"}, To: []string{"/renamed"},
		}},
	})
	if err != nil {
		t.Fatalf("NewManifest: %v", err)
	}
	projector, err := NewProjector(manifest)
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}

	values := []any{
		nil,
		true,
		"text",
		42,
		map[string]any{"nested": []any{"a", 2, false, nil}},
		[]any{map[string]any{"tag": "one"}, map[string]any{"tag": "two"}},
	}
	for _, value := range values {
		canonical := map[string]any{"value": value}
		projection, err := projector.Project(canonical)
		if err != nil {
			t.Fatalf("Project(%#v): %v", value, err)
		}
		reversed, err := projector.Reverse(projection.Document)
		if err != nil {
			t.Fatalf("Reverse(%#v): %v", value, err)
		}
		if !reflect.DeepEqual(reversed, canonical) {
			t.Fatalf("round trip(%#v) = %#v, want %#v", value, reversed, canonical)
		}
	}
}

func TestProjectorPreservesAbsentVersusNull(t *testing.T) {
	t.Parallel()

	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{{
			ID: "nullable", CanonicalPath: "/nullable", Classification: CoverageSupported, OwnedPaths: []string{"/old_nullable"},
		}},
		Transforms: []Transform{{
			ID: "nullable.presence", FactID: "nullable", Primitive: PrimitivePresence,
			From: []string{"/nullable"}, To: []string{"/old_nullable"},
		}},
	})
	if err != nil {
		t.Fatalf("NewManifest: %v", err)
	}
	projector, err := NewProjector(manifest)
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}

	absent, err := projector.Project(map[string]any{})
	if err != nil {
		t.Fatalf("Project(absent): %v", err)
	}
	if _, exists := absent.Document["old_nullable"]; exists {
		t.Fatalf("absent value became present: %#v", absent.Document)
	}
	explicitNull, err := projector.Project(map[string]any{"nullable": nil})
	if err != nil {
		t.Fatalf("Project(null): %v", err)
	}
	value, exists := explicitNull.Document["old_nullable"]
	if !exists || value != nil {
		t.Fatalf("explicit null = (%#v, %t), want (nil, true)", value, exists)
	}
}

func TestProjectorSupportsArrayPointers(t *testing.T) {
	t.Parallel()

	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{{
			ID: "first.tag", CanonicalPath: "/items", Classification: CoverageSupported, OwnedPaths: []string{"/outbounds"},
		}},
		Transforms: []Transform{{
			ID: "first.tag.rename", FactID: "first.tag", Primitive: PrimitiveRename,
			From: []string{"/items/0/name"}, To: []string{"/outbounds/0/tag"},
		}},
	})
	if err != nil {
		t.Fatalf("NewManifest: %v", err)
	}
	projector, err := NewProjector(manifest)
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}
	canonical := map[string]any{"items": []any{map[string]any{"name": "primary"}}}
	projection, err := projector.Project(canonical)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	want := map[string]any{"outbounds": []any{map[string]any{"tag": "primary"}}}
	if !reflect.DeepEqual(projection.Document, want) {
		t.Fatalf("array projection = %#v, want %#v", projection.Document, want)
	}
	result, err := projector.Reverse(projection.Document)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if !reflect.DeepEqual(result, canonical) {
		t.Fatalf("array round trip = %#v, want %#v", result, canonical)
	}
}

func TestProjectorReportsUnsupportedAndBehaviorChangedFacts(t *testing.T) {
	t.Parallel()

	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.11.0"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{
			{ID: "name", CanonicalPath: "/name", Classification: CoverageBehaviorChanged, OwnedPaths: []string{"/name"}},
			{ID: "future", CanonicalPath: "/future", Classification: CoverageIntentionallyUnsupported},
		},
		Transforms: []Transform{{
			ID: "name.rename", FactID: "name", Primitive: PrimitiveRename,
			From: []string{"/name"}, To: []string{"/name"},
		}},
	})
	if err != nil {
		t.Fatalf("NewManifest: %v", err)
	}
	projector, err := NewProjector(manifest)
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}
	projection, err := projector.Project(map[string]any{"name": "test", "future": true})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(projection.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v, want two", projection.Diagnostics)
	}
	if projection.Diagnostics[0].Code != "behavior_changed" || projection.Diagnostics[1].Code != "fact_omitted" {
		t.Fatalf("diagnostic codes = %q, %q, want behavior_changed and fact_omitted", projection.Diagnostics[0].Code, projection.Diagnostics[1].Code)
	}
}

func TestProjectorRejectsLossyPrimitiveInputs(t *testing.T) {
	t.Parallel()

	projector, err := NewProjector(allPrimitivesManifest(t))
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}
	tests := []struct {
		name      string
		canonical map[string]any
		version   map[string]any
	}{
		{
			name: "join part contains separator",
			canonical: map[string]any{"canonical": map[string]any{
				"join": map[string]any{"host": "host:alias", "port": "443"},
			}},
		},
		{
			name: "unwrap would discard sibling",
			canonical: map[string]any{"canonical": map[string]any{
				"unwrapped": map[string]any{"value": "safe", "unknown": true},
			}},
		},
		{
			name: "wrap reverse would discard sibling",
			version: map[string]any{"legacy": map[string]any{
				"wrapped": map[string]any{"value": "safe", "unknown": true},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.canonical != nil {
				if _, err := projector.Project(test.canonical); err == nil {
					t.Fatalf("Project() succeeded for lossy input")
				}
				return
			}
			if _, err := projector.Reverse(test.version); err == nil {
				t.Fatalf("Reverse() succeeded for lossy input")
			}
		})
	}
}

func TestProjectorReverseRejectsUnknownVersionPaths(t *testing.T) {
	t.Parallel()

	projector, err := NewProjector(allPrimitivesManifest(t))
	if err != nil {
		t.Fatalf("NewProjector: %v", err)
	}
	for _, unknown := range []any{true, map[string]any{}, []any{}} {
		_, err = projector.Reverse(map[string]any{
			"legacy":  map[string]any{"name": "known"},
			"unknown": unknown,
		})
		if err == nil {
			t.Fatalf("Reverse accepted unowned version value %#v", unknown)
		}
	}
}

func allPrimitivesManifest(t *testing.T) *Manifest {
	t.Helper()
	fact := func(id, canonical, owned string) SemanticFact {
		return SemanticFact{ID: id, CanonicalPath: canonical, Classification: CoverageSupported, OwnedPaths: []string{owned}}
	}
	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{
			fact("name", "/canonical/name", "/legacy/name"),
			fact("wrapped", "/canonical/wrapped", "/legacy/wrapped"),
			fact("unwrapped", "/canonical/unwrapped", "/legacy/unwrapped"),
			fact("split", "/canonical/split", "/legacy/split"),
			fact("join", "/canonical/join", "/legacy/join"),
			fact("mode", "/canonical/mode", "/legacy/mode"),
			fact("enabled", "/canonical/enabled", "/legacy/enabled"),
			fact("conditional", "/canonical/conditional", "/legacy/conditional"),
			fact("nullable", "/canonical/nullable", "/legacy/nullable"),
		},
		Transforms: []Transform{
			{ID: "name.rename", FactID: "name", Primitive: PrimitiveRename, From: []string{"/canonical/name"}, To: []string{"/legacy/name"}},
			{ID: "wrapped.wrap", FactID: "wrapped", Primitive: PrimitiveWrap, From: []string{"/canonical/wrapped"}, To: []string{"/legacy/wrapped"}, Key: "value"},
			{ID: "unwrapped.unwrap", FactID: "unwrapped", Primitive: PrimitiveUnwrap, From: []string{"/canonical/unwrapped"}, To: []string{"/legacy/unwrapped"}, Key: "value"},
			{ID: "split.split", FactID: "split", Primitive: PrimitiveSplit, From: []string{"/canonical/split"}, To: []string{"/legacy/split/host", "/legacy/split/port"}, Separator: ":"},
			{ID: "join.join", FactID: "join", Primitive: PrimitiveJoin, From: []string{"/canonical/join/host", "/canonical/join/port"}, To: []string{"/legacy/join"}, Separator: ":"},
			{ID: "mode.enum", FactID: "mode", Primitive: PrimitiveEnum, From: []string{"/canonical/mode"}, To: []string{"/legacy/mode"}, Enum: map[string]string{"direct": "direct", "block": "reject"}},
			{ID: "enabled.presence", FactID: "enabled", Primitive: PrimitivePresence, From: []string{"/canonical/enabled"}, To: []string{"/legacy/enabled"}},
			{ID: "conditional.copy", FactID: "conditional", Primitive: PrimitiveConditional, From: []string{"/canonical/conditional/value"}, To: []string{"/legacy/conditional/value"}, When: &Condition{CanonicalPath: "/canonical/enabled", VersionPath: "/legacy/enabled", Equals: true}},
			{ID: "nullable.presence", FactID: "nullable", Primitive: PrimitivePresence, From: []string{"/canonical/nullable"}, To: []string{"/legacy/nullable"}},
		},
	})
	if err != nil {
		t.Fatalf("NewManifest(all primitives): %v", err)
	}
	return manifest
}
