// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestReplaceCanonicalUsesCASAndNoOpDetection(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	application.now = func() time.Time { return time.Date(2026, time.August, 26, 1, 2, 3, 0, time.UTC) }
	sequence := byte(0)
	application.random = func(destination []byte) (int, error) {
		sequence++
		for index := range destination {
			destination[index] = sequence
		}
		return len(destination), nil
	}
	document := configuration.EmptyV2().CanonicalJSON()

	first, err := application.ReplaceCanonical(ctx, "", document)
	if err != nil {
		t.Fatalf("ReplaceCanonical() error = %v", err)
	}
	if first.Revision.Sequence != 1 || first.TaskID == "" || first.NoChange {
		t.Fatalf("first save = %+v", first)
	}

	noChange, err := application.ReplaceCanonical(ctx, first.Revision.ID, document)
	if err != nil {
		t.Fatalf("no-op ReplaceCanonical() error = %v", err)
	}
	if !noChange.NoChange || noChange.TaskID != "" || noChange.Revision.ID != first.Revision.ID {
		t.Fatalf("no-op save = %+v", noChange)
	}

	_, err = application.ReplaceCanonical(ctx, "", document)
	if !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("stale ReplaceCanonical() error = %v", err)
	}
}

func TestRevisionHistoryDiffRestoreAndTaskControl(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)

	initial, err := application.ReplaceCanonical(ctx, "", configuration.EmptyV2().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := application.SetCanonicalValue(
		ctx, initial.Revision.ID, "/configuration/log", []byte(`{"level":"info"}`),
	)
	if err != nil {
		t.Fatal(err)
	}

	page, err := application.ListCanonicalRevisions(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != changed.Revision.ID || page.Items[1].ID != initial.Revision.ID {
		t.Fatalf("revision page = %+v", page)
	}
	bySequence, err := application.CanonicalRevision(ctx, "#1")
	if err != nil || bySequence.ID != initial.Revision.ID {
		t.Fatalf("CanonicalRevision(#1) = %+v, %v", bySequence, err)
	}
	diff, err := application.DiffCanonicalRevisions(ctx, "#1", changed.Revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) != 1 || diff.Changes[0].Path != "/configuration/log" {
		t.Fatalf("revision diff = %+v", diff.Changes)
	}
	restored, err := application.RestoreCanonicalRevision(ctx, changed.Revision.ID, "#1")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Revision.Sequence != 3 || restored.Revision.ParentID != changed.Revision.ID {
		t.Fatalf("restored revision = %+v", restored.Revision)
	}

	task, err := application.Task(ctx, initial.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != store.TaskStatusQueued || task.Terminal() {
		t.Fatalf("queued task = %+v", task)
	}
	canceled, err := application.CancelTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status != store.TaskStatusCanceled || !canceled.Terminal() {
		t.Fatalf("canceled task = %+v", canceled)
	}
}

func TestResolveCoreVersionUsesExplicitOrActualRunningIdentityOnly(t *testing.T) {
	application := &Application{runtime: fakeRuntimeResolver{identity: RuntimeIdentity{
		PID: 77, ExactCoreVersion: "1.12.9", CoreArtifactID: "core-a", ActivationBundleID: "bundle-a",
	}}}
	explicit, err := application.ResolveCoreVersion(context.Background(), "1.13.19")
	if err != nil || explicit.ExactVersion != "1.13.19" || explicit.Source != "explicit" || explicit.Running != nil {
		t.Fatalf("explicit resolution=%+v err=%v", explicit, err)
	}
	running, err := application.ResolveCoreVersion(context.Background(), "")
	if err != nil || running.ExactVersion != "1.12.9" || running.Source != "running" || running.Running == nil || running.Running.PID != 77 {
		t.Fatalf("running resolution=%+v err=%v", running, err)
	}

	application.runtime = fakeRuntimeResolver{err: ErrNoRunningCore}
	if _, err := application.ResolveCoreVersion(context.Background(), ""); !errors.Is(err, ErrNoRunningCore) {
		t.Fatalf("omitted resolution error=%v", err)
	}
	if _, err := application.ResolveCoreVersion(context.Background(), "latest"); err == nil {
		t.Fatal("explicit resolution accepted a non-exact version")
	}
}

type fakeRuntimeResolver struct {
	identity RuntimeIdentity
	err      error
}

func (resolver fakeRuntimeResolver) Resolve(context.Context) (RuntimeIdentity, error) {
	return resolver.identity, resolver.err
}
