// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"reflect"
	"testing"
)

func TestReversePartialKeepsUnknownFieldsResidual(t *testing.T) {
	manifest, err := NewManifest(ManifestSpec{
		SchemaVersion: ManifestSchemaVersion,
		CoreVersion:   version(t, "1.13.19"),
		SupportLevel:  SupportNativeStructured,
		SemanticFacts: []SemanticFact{{
			ID: "log.level", CanonicalPath: "/global/log/level",
			Classification: CoverageSupported, OwnedPaths: []string{"/log/level"},
		}},
		Transforms: []Transform{{
			ID: "log.level.rename", FactID: "log.level", Primitive: PrimitiveRename,
			From: []string{"/global/log/level"}, To: []string{"/log/level"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projector, err := NewProjector(manifest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := projector.ReversePartial(map[string]any{
		"log":          map[string]any{"level": "warn", "timestamp": true},
		"experimental": map[string]any{"future": []any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCanonical := map[string]any{"global": map[string]any{"log": map[string]any{"level": "warn"}}}
	if !reflect.DeepEqual(result.Canonical, wantCanonical) {
		t.Fatalf("canonical = %#v, want %#v", result.Canonical, wantCanonical)
	}
	wantResidual := []string{"/experimental/future", "/log/timestamp"}
	if !reflect.DeepEqual(result.ResidualPaths, wantResidual) {
		t.Fatalf("residual = %#v, want %#v", result.ResidualPaths, wantResidual)
	}
}
