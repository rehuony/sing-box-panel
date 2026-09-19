// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestConfigurationFilePreservesInvalidTextAndLegacyHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := FromStore(db)
	initial, err := app.ReplaceConfiguration(ctx, "", []byte(`{"x":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	file, err := app.ConfigurationFile(ctx)
	if err != nil || file.CanonicalRevisionID != initial.Revision.ID || file.Revision != 1 {
		t.Fatalf("initial: %+v %v", file, err)
	}
	raw := "{\n  \"x\": 9007199254740993,\n"
	saved, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: file.Revision, Content: raw})
	if err != nil || saved.Content != raw || saved.SyntaxValid || saved.CanonicalRevisionID != "" {
		t.Fatalf("invalid save: %+v %v", saved, err)
	}
	head, err := db.Head(ctx)
	if err != nil || head.ID != initial.Revision.ID {
		t.Fatalf("immutable history changed: %+v %v", head, err)
	}
	if _, err := app.ReplaceConfiguration(ctx, head.ID, []byte(`{}`)); !errors.Is(err, store.ErrConfigurationFileUnparsed) {
		t.Fatalf("legacy overwrote unparsed file: %v", err)
	}
	if _, err := app.PatchConfiguration(ctx, head.ID, []CanonicalChange{{Operation: "set", Path: "/x", ValueJSON: "1"}}); !errors.Is(err, store.ErrConfigurationFileUnparsed) {
		t.Fatalf("patch overwrote unparsed file: %v", err)
	}
	if _, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: file.Revision, Content: "{}"}); !errors.Is(err, store.ErrConfigurationFileConflict) {
		t.Fatalf("CAS: %v", err)
	}
	db2, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	reloaded, err := FromStore(db2).ConfigurationFile(ctx)
	if err != nil || reloaded.Content != raw || reloaded.Revision != saved.Revision {
		t.Fatalf("reload: %+v %v", reloaded, err)
	}
	corrected := "{\n  \"x\": 9007199254740993\n}\n"
	fixed, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: saved.Revision, Content: corrected})
	if err != nil || !fixed.SyntaxValid || fixed.Content != corrected || fixed.CanonicalRevisionID != initial.Revision.ID {
		t.Fatalf("correction: %+v %v", fixed, err)
	}
	noChange, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: fixed.Revision, Content: corrected})
	if err != nil || noChange.Revision != fixed.Revision {
		t.Fatalf("no change: %+v %v", noChange, err)
	}
	replaced, err := app.ReplaceConfiguration(ctx, fixed.CanonicalRevisionID, []byte(`{"x":9007199254740994}`))
	if err != nil {
		t.Fatal(err)
	}
	file, err = app.ConfigurationFile(ctx)
	if err != nil || file.CanonicalRevisionID != replaced.Revision.ID || file.Revision != fixed.Revision+1 {
		t.Fatalf("legacy sync: %+v %v", file, err)
	}
}

func TestConfigurationFileConcurrentSavesHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := FromStore(db)
	var wg sync.WaitGroup
	errorsByWriter := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, errorsByWriter[i] = app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Content: []string{"{", "["}[i]})
		})
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errorsByWriter {
		if err == nil {
			successes++
		} else if errors.Is(err, store.ErrConfigurationFileConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}
