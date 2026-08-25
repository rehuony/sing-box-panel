// SPDX-License-Identifier: GPL-3.0-or-later

package reconcile

import (
	"errors"
	"testing"
)

func TestThreeWayMergesIndependentChangesAndReportsPresenceConflict(t *testing.T) {
	base := map[string]any{"log": map[string]any{"level": "info"}, "remove": "base"}
	current := map[string]any{"log": map[string]any{"level": "warn"}, "remove": "current", "current": true}
	manual := map[string]any{"log": map[string]any{"level": "debug"}, "manual": true}

	preview, err := ThreeWay(base, current, manual)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Conflicts) != 2 {
		t.Fatalf("conflicts = %#v, want level and removal conflicts", preview.Conflicts)
	}
	if preview.Conflicts[0].Path != "/log/level" || preview.Conflicts[1].Path != "/remove" {
		t.Fatalf("conflict order/paths = %#v", preview.Conflicts)
	}
	if preview.Conflicts[1].Manual.Present {
		t.Fatalf("manual removal presence = %#v, want absent", preview.Conflicts[1].Manual)
	}
	if preview.Merged["current"] != true || preview.Merged["manual"] != true {
		t.Fatalf("independent additions were not merged: %#v", preview.Merged)
	}
}

func TestResolveRequiresEveryAndOnlyCurrentConflict(t *testing.T) {
	base := map[string]any{"value": "base"}
	current := map[string]any{"value": "current"}
	manual := map[string]any{"value": "manual"}
	if _, err := Resolve(base, current, manual, nil); !errors.Is(err, ErrUnresolvedConflict) {
		t.Fatalf("Resolve() error = %v", err)
	}
	resolved, err := Resolve(base, current, manual, map[string]Choice{"/value": ChooseManual})
	if err != nil {
		t.Fatal(err)
	}
	if resolved["value"] != "manual" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if _, err := Resolve(base, current, manual, map[string]Choice{"/value": ChooseCurrent, "/stale": ChooseManual}); err == nil {
		t.Fatal("Resolve() accepted an extra stale decision")
	}
}

func TestThreeWayTreatsArraysAsOrderedAtomicValues(t *testing.T) {
	base := map[string]any{"nodes": []any{"a", "b"}}
	current := map[string]any{"nodes": []any{"b", "a"}}
	manual := map[string]any{"nodes": []any{"a", "c"}}
	preview, err := ThreeWay(base, current, manual)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Conflicts) != 1 || preview.Conflicts[0].Path != "/nodes" {
		t.Fatalf("array conflicts = %#v", preview.Conflicts)
	}
}
