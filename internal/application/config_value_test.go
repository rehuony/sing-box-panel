// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestCanonicalPatchPreservesNumericLexemesAndAdvancesOnce(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	initial, err := app.ReplaceCanonical(ctx, "", []byte(
		`{"experimental":{"large":9007199254740993,"huge":1e999,"decimal":1.0,"scalar":"leaf"}}`,
	))
	if err != nil {
		t.Fatal(err)
	}

	saved, err := app.PatchCanonical(ctx, initial.Revision.ID, []CanonicalChange{
		{Operation: "set", Path: "/experimental/note", ValueJSON: `"unrelated edit"`},
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
		{Operation: "set", Path: "/experimental/large", ValueJSON: `9007199254740995`},
		{Operation: "set", Path: "/experimental/payload", ValueJSON: `{"long":9007199254740997,"huge":1e999,"decimal":1.0}`},
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
		{Operation: "set", Path: "/experimental/stale", ValueJSON: `true`},
	}); !IsRevisionConflict(err) {
		t.Fatalf("stale patch error = %v", err)
	}
	if _, err := app.PatchCanonical(ctx, updated.Revision.ID, []CanonicalChange{
		{Operation: "set", Path: "/experimental/scalar/child", ValueJSON: `true`},
	}); !errors.Is(err, ErrCanonicalPatchInvalid) {
		t.Fatalf("scalar-crossing patch error = %v", err)
	}
	head, err := app.CanonicalHead(ctx)
	if err != nil || head == nil || head.ID != updated.Revision.ID {
		t.Fatalf("head after rejected patch = %+v, err=%v", head, err)
	}
}
