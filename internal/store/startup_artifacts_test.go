// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStartupArtifactLifecycleAndManualStaleness(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-a", 501, 'a', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	first, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-a", SchemaVersion: 1, Document: json.RawMessage(`{"value":1}`), CommandID: "command-a", CreatedAt: now,
	}, NewTask{ID: "revision-task-a", Lane: TaskLaneMaintenance, Kind: "canonical-saved"})
	if err != nil {
		t.Fatal(err)
	}

	raw := []byte("{\n  // exact bytes\n  \"log\": {},\n}\n")
	created, err := database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-a", Kind: StartupArtifactManual, CanonicalRevisionID: first.ID,
		ExactCoreVersion: "1.13.19", RendererVersion: "manual-v1", CoreArtifactID: core.ID,
		ConfigBytes: raw, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.State != StartupArtifactPending || created.ConfigSHA256 == "" || string(created.ConfigBytes) != string(raw) {
		t.Fatalf("created startup = %+v", created)
	}
	created.ConfigBytes[0] = 'x'
	stored, err := database.GetStartupArtifact(ctx, created.ID)
	if err != nil || string(stored.ConfigBytes) != string(raw) {
		t.Fatalf("stored exact bytes=%q err=%v", stored.ConfigBytes, err)
	}
	ready, err := database.CompleteStartupArtifactCheck(ctx, created.ID, true, json.RawMessage(`[{"code":"ok"}]`), now.Add(2*time.Second))
	if err != nil || ready.State != StartupArtifactReady || ready.CheckedAt == nil {
		t.Fatalf("ready startup=%+v err=%v", ready, err)
	}

	second, err := database.SaveCanonicalRevisionAndTask(ctx, first.ID, NewCanonicalRevision{
		ID: "revision-b", SchemaVersion: 1, Document: json.RawMessage(`{"value":2}`), CommandID: "command-b", CreatedAt: now.Add(3 * time.Second),
	}, NewTask{ID: "revision-task-b", Lane: TaskLaneMaintenance, Kind: "canonical-saved"})
	if err != nil || second.Sequence != 2 {
		t.Fatalf("second revision=%+v err=%v", second, err)
	}
	stale, err := database.GetStartupArtifact(ctx, created.ID)
	if err != nil || stale.State != StartupArtifactStale {
		t.Fatalf("stale startup=%+v err=%v", stale, err)
	}
	late, err := database.CompleteStartupArtifactCheck(ctx, created.ID, true, nil, now.Add(4*time.Second))
	if err != nil || late.State != StartupArtifactStale {
		t.Fatalf("late completion=%+v err=%v", late, err)
	}
}

func TestStartupArtifactRejectsVersionAndCapabilityMismatches(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 11, 0, 0, 0, time.UTC)
	core := testCoreArtifact("core-a", 601, 'b', "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-a", SchemaVersion: 1, Document: json.RawMessage(`{}`), CommandID: "command-a", CreatedAt: now,
	}, NewTask{ID: "task-a", Lane: TaskLaneMaintenance, Kind: "canonical-saved"})
	if err != nil {
		t.Fatal(err)
	}
	base := StartupArtifact{
		ID: "startup-a", Kind: StartupArtifactStructured, CanonicalRevisionID: revision.ID,
		ExactCoreVersion: "1.13.19", RendererVersion: "renderer-v1", CoreArtifactID: core.ID,
		CapabilityCommit: strings.Repeat("c", 40), CapabilityDigest: strings.Repeat("d", 64), ConfigBytes: []byte(`{}`),
	}
	missingCapability := base
	missingCapability.ID = "missing-capability"
	missingCapability.CapabilityCommit = ""
	if _, err := database.CreateStartupArtifact(ctx, missingCapability); err == nil {
		t.Fatal("structured artifact accepted a missing capability commit")
	}
	wrongVersion := base
	wrongVersion.ID = "wrong-version"
	wrongVersion.ExactCoreVersion = "1.12.0"
	if _, err := database.CreateStartupArtifact(ctx, wrongVersion); err == nil {
		t.Fatal("startup artifact accepted a version/core mismatch")
	}
	created, err := database.CreateStartupArtifact(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateStartupArtifact(ctx, base); !errors.Is(err, ErrStartupArtifactExists) {
		t.Fatalf("duplicate startup error=%v", err)
	}
	page, err := database.ListStartupArtifacts(ctx, StartupArtifactListFilter{State: StartupArtifactPending, Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("startup page=%+v err=%v", page, err)
	}
}

func TestCatalogStateAndSharedContentArtifacts(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	first := testCoreArtifact("core-a", 701, 'e', "amd64", now)
	second := first
	second.ID = "core-b"
	second.AssetID = 702
	// One content-addressed file may legitimately be referenced by multiple
	// source identities.
	for _, artifact := range []CoreArtifact{first, second} {
		if _, err := database.UpsertCoreArtifact(ctx, artifact); err != nil {
			t.Fatalf("shared content artifact %s: %v", artifact.ID, err)
		}
	}
	state, err := database.SaveCatalogState(ctx, CatalogState{
		Validator: "validator", Catalog: json.RawMessage(`{"releases":[]}`),
		Diagnostics: json.RawMessage(`[{"code":"fresh"}]`), RefreshedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	state.Catalog[0] = 'x'
	loaded, err := database.CatalogState(ctx)
	if err != nil || string(loaded.Catalog) != `{"releases":[]}` || !loaded.RefreshedAt.Equal(now) {
		t.Fatalf("loaded catalog=%+v err=%v", loaded, err)
	}
}
