// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestInitializeConfigurationFileCreatesOnlyMissingDocument(t *testing.T) {
	for _, test := range []struct {
		name    string
		saved   bool
		content string
	}{
		{name: "missing"},
		{name: "saved empty object", saved: true, content: "{}"},
		{name: "saved configuration", saved: true, content: "{\n  \"log\": {\"level\": \"debug\"}\n}\n"},
		{name: "unfinished JSON", saved: true, content: "{\n  \"inbounds\":"},
		{name: "blank draft", saved: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			app := FromStore(db)
			if test.saved {
				if _, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Content: test.content}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := db.ConfigurationFile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for attempt := range 2 {
				if err := app.InitializeConfigurationFile(ctx); err != nil {
					t.Fatal(err)
				}
				after, err := db.ConfigurationFile(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if test.saved || attempt > 0 {
					if after != before {
						t.Fatalf("initialization changed saved configuration: before=%+v after=%+v", before, after)
					}
				} else {
					if after.Content != "{}" || after.Revision != 1 || after.CanonicalRevisionID == "" || after.UpdatedAt.IsZero() {
						t.Fatalf("empty configuration was not persisted: %+v", after)
					}
					head, err := db.Head(ctx)
					if err != nil || head == nil || head.ID != after.CanonicalRevisionID || head.Sequence != 1 || string(head.Document) != "{}" {
						t.Fatalf("initial canonical revision: %+v %v", head, err)
					}
				}
				before = after
			}
		})
	}
}

func TestInitializeConfigurationFilePreservesConcurrentSave(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := FromStore(db)
	other := FromStore(db)
	var saved ConfigurationFile
	readRandom := app.random
	app.random = func(bytes []byte) (int, error) {
		if saved.Revision == 0 {
			var err error
			saved, err = other.SaveConfigurationFile(ctx, ConfigurationFileWrite{Content: "{\n"})
			if err != nil {
				t.Fatal(err)
			}
		}
		return readRandom(bytes)
	}
	if err := app.InitializeConfigurationFile(ctx); err != nil {
		t.Fatal(err)
	}
	file, err := app.ConfigurationFile(ctx)
	if err != nil || !reflect.DeepEqual(file, saved) {
		t.Fatalf("concurrent save was replaced: got=%+v want=%+v err=%v", file, saved, err)
	}
}

func TestInitializeConfigurationFilePropagatesSaveFailure(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := FromStore(db)
	want := errors.New("random source unavailable")
	app.random = func([]byte) (int, error) { return 0, want }
	if err := app.InitializeConfigurationFile(ctx); !errors.Is(err, want) {
		t.Fatalf("initialization error=%v, want %v", err, want)
	}
	file, err := app.ConfigurationFile(ctx)
	if err != nil || file.Revision != 0 || file.CanonicalRevisionID != "" {
		t.Fatalf("failed initialization saved configuration: %+v %v", file, err)
	}
}

func TestConfigurationFilePreservesInvalidTextAndRuntimeSnapshots(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := FromStore(db)
	initial, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Content: `{"x":9007199254740993}`})
	if err != nil {
		t.Fatal(err)
	}
	file, err := app.ConfigurationFile(ctx)
	if err != nil || file.CanonicalRevisionID != initial.CanonicalRevisionID || file.Revision != 1 {
		t.Fatalf("initial: %+v %v", file, err)
	}
	raw := "{\n  \"x\": 9007199254740993,\n"
	saved, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: file.Revision, Content: raw})
	if err != nil || saved.Content != raw || saved.SyntaxValid || saved.CanonicalRevisionID != "" {
		t.Fatalf("invalid save: %+v %v", saved, err)
	}
	head, err := db.Head(ctx)
	if err != nil || head.ID != initial.CanonicalRevisionID {
		t.Fatalf("immutable history changed: %+v %v", head, err)
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
	if err != nil || !fixed.SyntaxValid || fixed.Content != corrected || fixed.CanonicalRevisionID != initial.CanonicalRevisionID {
		t.Fatalf("correction: %+v %v", fixed, err)
	}
	noChange, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: fixed.Revision, Content: corrected})
	if err != nil || noChange.Revision != fixed.Revision {
		t.Fatalf("no change: %+v %v", noChange, err)
	}
	replaced, err := app.SaveConfigurationFile(ctx, ConfigurationFileWrite{Revision: fixed.Revision, Content: `{"x":9007199254740994}`})
	if err != nil {
		t.Fatal(err)
	}
	file, err = app.ConfigurationFile(ctx)
	if err != nil || file.CanonicalRevisionID != replaced.CanonicalRevisionID || file.Revision != fixed.Revision+1 {
		t.Fatalf("saved snapshot: %+v %v", file, err)
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
