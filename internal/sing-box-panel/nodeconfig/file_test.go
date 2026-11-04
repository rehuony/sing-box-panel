// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestConfigurationFilePreservesInvalidTextAndRuntimeSnapshots(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := testutil.CreateNode(t, db, "edge")
	defer db.Close()
	app := New(db, nil)
	initial, err := app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Content: `{"x":9007199254740993}`})
	if err != nil {
		t.Fatal(err)
	}
	file, err := app.ConfigurationFile(ctx, nodeID)
	if err != nil || file.CanonicalRevisionID != initial.CanonicalRevisionID || file.Revision != 1 {
		t.Fatalf("initial: %+v %v", file, err)
	}
	raw := "{\n  \"x\": 9007199254740993,\n"
	saved, err := app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Revision: file.Revision, Content: raw})
	if err != nil || saved.Content != raw || saved.SyntaxValid || saved.CanonicalRevisionID != "" {
		t.Fatalf("invalid save: %+v %v", saved, err)
	}
	head, err := db.Head(ctx, nodeID)
	if err != nil || head.ID != initial.CanonicalRevisionID {
		t.Fatalf("immutable history changed: %+v %v", head, err)
	}
	if _, err := app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Revision: file.Revision, Content: "{}"}); !errors.Is(err, store.ErrConfigurationFileConflict) {
		t.Fatalf("CAS: %v", err)
	}
	db2, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	reloaded, err := New(db2, nil).ConfigurationFile(ctx, nodeID)
	if err != nil || reloaded.Content != raw || reloaded.Revision != saved.Revision {
		t.Fatalf("reload: %+v %v", reloaded, err)
	}
	corrected := "{\n  \"x\": 9007199254740993\n}\n"
	fixed, err := app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Revision: saved.Revision, Content: corrected})
	if err != nil || !fixed.SyntaxValid || fixed.Content != corrected || fixed.CanonicalRevisionID != initial.CanonicalRevisionID {
		t.Fatalf("correction: %+v %v", fixed, err)
	}
	noChange, err := app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Revision: fixed.Revision, Content: corrected})
	if err != nil || noChange.Revision != fixed.Revision {
		t.Fatalf("no change: %+v %v", noChange, err)
	}
	replaced, err := app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Revision: fixed.Revision, Content: `{"x":9007199254740994}`})
	if err != nil {
		t.Fatal(err)
	}
	file, err = app.ConfigurationFile(ctx, nodeID)
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
	nodeID := testutil.CreateNode(t, db, "edge")
	defer db.Close()
	app := New(db, nil)
	var wg sync.WaitGroup
	errorsByWriter := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, errorsByWriter[i] = app.SaveConfigurationFile(ctx, nodeID, ConfigurationFileWrite{Content: []string{"{", "["}[i]})
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
