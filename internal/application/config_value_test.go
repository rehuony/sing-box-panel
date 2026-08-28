// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCanonicalPointerApplicationUsesRevisionCAS(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	initial, err := app.ReplaceCanonical(ctx, "", canonical.EmptyV2().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.SetCanonicalValue(ctx, initial.Revision.ID, "/configuration/log", []byte(`{"level":"warn"}`))
	if err != nil {
		t.Fatal(err)
	}
	value, err := app.CanonicalValueAt(ctx, "/configuration/log/level")
	if err != nil || value.Value != "warn" || value.Revision.ID != saved.Revision.ID {
		t.Fatalf("value=%+v err=%v", value, err)
	}
	if _, err := app.UnsetCanonicalValue(ctx, initial.Revision.ID, "/configuration/log"); !IsRevisionConflict(err) {
		t.Fatalf("stale unset error = %v", err)
	}
	removed, err := app.UnsetCanonicalValue(ctx, saved.Revision.ID, "/configuration/log")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CanonicalValueAt(ctx, "/configuration/log"); !errors.Is(err, canonical.ErrPointerNotFound) {
		t.Fatalf("removed pointer error = %v", err)
	}
	if removed.Revision.ID == saved.Revision.ID {
		t.Fatal("unset did not create a revision")
	}
}

func TestCanonicalPatchPreservesNumericLexemesAndAdvancesOnce(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	initial, err := app.ReplaceCanonical(ctx, "", []byte(
		`{"schema_version":2,"configuration":{"experimental":{"large":9007199254740993,"huge":1e999,"decimal":1.0,"scalar":"leaf"}}}`,
	))
	if err != nil {
		t.Fatal(err)
	}

	saved, err := app.PatchCanonical(ctx, initial.Revision.ID, []CanonicalChange{
		{Operation: "set", Path: "/configuration/experimental/note", ValueJSON: `"unrelated edit"`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision.Sequence != initial.Revision.Sequence+1 {
		t.Fatalf("sequence = %d, want %d", saved.Revision.Sequence, initial.Revision.Sequence+1)
	}
	for _, exact := range []string{
		`"large":9007199254740993`,
		`"huge":1e999`,
		`"decimal":1.0`,
		`"note":"unrelated edit"`,
	} {
		if !strings.Contains(saved.Revision.DocumentJSON, exact) {
			t.Fatalf("document_json lost %s: %s", exact, saved.Revision.DocumentJSON)
		}
	}

	updated, err := app.PatchCanonical(ctx, saved.Revision.ID, []CanonicalChange{
		{Operation: "set", Path: "/configuration/experimental/large", ValueJSON: `9007199254740995`},
		{Operation: "set", Path: "/configuration/experimental/payload", ValueJSON: `{"long":9007199254740997,"huge":1e999,"decimal":1.0}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		`"large":9007199254740995`,
		`"payload":{"decimal":1.0,"huge":1e999,"long":9007199254740997}`,
	} {
		if !strings.Contains(updated.Revision.DocumentJSON, exact) {
			t.Fatalf("document_json lost %s: %s", exact, updated.Revision.DocumentJSON)
		}
	}

	if _, err := app.PatchCanonical(ctx, saved.Revision.ID, []CanonicalChange{
		{Operation: "set", Path: "/configuration/experimental/stale", ValueJSON: `true`},
	}); !IsRevisionConflict(err) {
		t.Fatalf("stale patch error = %v", err)
	}
	if _, err := app.PatchCanonical(ctx, updated.Revision.ID, []CanonicalChange{
		{Operation: "set", Path: "/configuration/experimental/scalar/child", ValueJSON: `true`},
	}); !errors.Is(err, ErrCanonicalPatchInvalid) {
		t.Fatalf("scalar-crossing patch error = %v", err)
	}
	head, err := app.CanonicalHead(ctx)
	if err != nil || head == nil || head.ID != updated.Revision.ID {
		t.Fatalf("head after rejected patch = %+v, err=%v", head, err)
	}
}
