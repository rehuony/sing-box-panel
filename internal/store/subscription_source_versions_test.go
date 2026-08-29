// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSubscriptionSourceVersionsAppendReuseAndRestore(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC)
	source, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-versioned", Name: "versioned", SourceKind: SubscriptionSourceRemote,
		Config: json.RawMessage(`{"url":"https://example.test/sub"}`), Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	nodesV1 := json.RawMessage(`[{"key":"source:source-versioned:one","source_id":"source-versioned","type":"socks","tag":"one","outbound":{"type":"socks","tag":"one","server":"one.example","server_port":1080}}]`)
	first, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: SubscriptionSourceVersion{
			ID: "version-one", SourceID: source.ID, Format: "sing-box-json",
			RawBody: []byte(`{"outbounds":[{"tag":"one"}]}`), NormalizedNodes: nodesV1,
			Diagnostics: json.RawMessage(`[]`), FetchedAt: now.Add(time.Minute), CreatedAt: now.Add(time.Minute),
		},
		ExpectedSourceUpdatedAt: source.UpdatedAt, UpdatedAt: now.Add(time.Minute),
	})
	if err != nil || first.Source.CurrentVersionID != first.Version.ID {
		t.Fatalf("first=%+v err=%v", first, err)
	}

	nodesV2 := json.RawMessage(`[{"key":"source:source-versioned:two","source_id":"source-versioned","type":"socks","tag":"two","outbound":{"type":"socks","tag":"two","server":"two.example","server_port":1080}}]`)
	second, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: SubscriptionSourceVersion{
			ID: "version-two", SourceID: source.ID, Format: "uri-list",
			RawBody: []byte(`socks://two.example:1080`), NormalizedNodes: nodesV2,
			FetchedAt: now.Add(2 * time.Minute), CreatedAt: now.Add(2 * time.Minute),
		},
		ExpectedSourceUpdatedAt: first.Source.UpdatedAt, UpdatedAt: now.Add(2 * time.Minute),
	})
	if err != nil || second.Source.CurrentVersionID != second.Version.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}

	page, err := database.ListSubscriptionSourceVersions(ctx, SubscriptionSourceVersionListFilter{SourceID: source.ID})
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != second.Version.ID || page.Items[1].ID != first.Version.ID {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	loaded, err := database.GetSubscriptionSourceVersion(ctx, source.ID, first.Version.ID)
	if err != nil || string(loaded.NormalizedNodes) != string(first.Version.NormalizedNodes) {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	restored, err := database.ActivateSubscriptionSourceVersion(
		ctx, source.ID, first.Version.ID, second.Source.UpdatedAt, now.Add(3*time.Minute),
	)
	if err != nil || restored.CurrentVersionID != first.Version.ID {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}

	reused, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: SubscriptionSourceVersion{
			ID: "version-duplicate-body", SourceID: source.ID, Format: "sing-box-json",
			RawBody: first.Version.RawBody, NormalizedNodes: first.Version.NormalizedNodes,
			FetchedAt: now.Add(4 * time.Minute), CreatedAt: now.Add(4 * time.Minute),
		},
		ExpectedSourceUpdatedAt: restored.UpdatedAt, UpdatedAt: now.Add(4 * time.Minute),
	})
	if err != nil || reused.Version.ID != first.Version.ID || reused.Source.CurrentVersionID != first.Version.ID {
		t.Fatalf("reused=%+v err=%v", reused, err)
	}
	page, err = database.ListSubscriptionSourceVersions(ctx, SubscriptionSourceVersionListFilter{SourceID: source.ID})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("versions after digest reuse=%+v err=%v", page, err)
	}
}

func TestSubscriptionSourceAndVersionWritesRollBackWhenRefreshTaskInsertFails(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	collision := EnqueueTaskInput{
		ID: "task-refresh-collision", IdempotencyKey: "existing-refresh-task",
		Lane: TaskLaneMaintenance, Kind: TaskKindSubscriptionSourceRefresh,
		Payload: json.RawMessage(`{"source_id":"unrelated"}`), CreatedAt: now,
	}
	if _, err := database.EnqueueTask(ctx, collision); err != nil {
		t.Fatal(err)
	}
	refreshTask := collision
	refreshTask.IdempotencyKey = "new-refresh-task"
	refreshTask.Payload = json.RawMessage(`{"source_id":"source-atomic"}`)

	if _, err := database.CreateSubscriptionSourceAndTask(ctx, SubscriptionSource{
		ID: "source-create-rollback", Name: "create rollback", SourceKind: SubscriptionSourceRemote,
		Config: json.RawMessage(`{"url":"https://example.test/create"}`), Enabled: true, CreatedAt: now,
	}, &refreshTask); err == nil {
		t.Fatal("source create succeeded despite refresh task identity collision")
	}
	if _, err := database.GetSubscriptionSource(ctx, "source-create-rollback"); !errors.Is(err, ErrSubscriptionSourceNotFound) {
		t.Fatalf("failed create left a source behind: %v", err)
	}

	source, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-atomic", Name: "atomic", SourceKind: SubscriptionSourceRemote,
		Config: json.RawMessage(`{"url":"https://example.test/original"}`), Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateSubscriptionSource(ctx, UpdateSubscriptionSourceInput{
		ID: source.ID, Name: "mutated", SourceKind: source.SourceKind,
		Config: json.RawMessage(`{"url":"https://example.test/mutated"}`), Enabled: source.Enabled,
		ExpectedUpdatedAt: source.UpdatedAt, UpdatedAt: now.Add(time.Minute), RefreshTask: &refreshTask,
	}); err == nil {
		t.Fatal("source update succeeded despite refresh task identity collision")
	}
	unchanged, err := database.GetSubscriptionSource(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != source.Name || !unchanged.UpdatedAt.Equal(source.UpdatedAt) ||
		string(unchanged.Config) != string(source.Config) {
		t.Fatalf("failed update partially persisted source: before=%+v after=%+v", source, unchanged)
	}

	nodesV1 := json.RawMessage(`[{"key":"source:source-atomic:one","source_id":"source-atomic","type":"socks","tag":"one","outbound":{"type":"socks","tag":"one","server":"one.example","server_port":1080}}]`)
	failedVersion := SubscriptionSourceVersion{
		ID: "version-rollback", SourceID: source.ID, Format: "sing-box-json",
		RawBody: []byte(`{"outbounds":[{"tag":"one"}]}`), NormalizedNodes: nodesV1,
		Diagnostics: json.RawMessage(`[]`), FetchedAt: now.Add(time.Minute), CreatedAt: now.Add(time.Minute),
	}
	if _, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: failedVersion, ExpectedSourceUpdatedAt: source.UpdatedAt,
		UpdatedAt: now.Add(time.Minute), RefreshTask: &refreshTask,
	}); err == nil {
		t.Fatal("version save succeeded despite refresh task identity collision")
	}
	if _, err := database.GetSubscriptionSourceVersion(ctx, source.ID, failedVersion.ID); !errors.Is(err, ErrSubscriptionSourceVersionNotFound) {
		t.Fatalf("failed version save left a version behind: %v", err)
	}
	unchanged, err = database.GetSubscriptionSource(ctx, source.ID)
	if err != nil || unchanged.CurrentVersionID != "" || !unchanged.UpdatedAt.Equal(source.UpdatedAt) {
		t.Fatalf("failed version save changed source pointer: source=%+v err=%v", unchanged, err)
	}

	first, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: failedVersion, ExpectedSourceUpdatedAt: source.UpdatedAt, UpdatedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	nodesV2 := json.RawMessage(`[{"key":"source:source-atomic:two","source_id":"source-atomic","type":"socks","tag":"two","outbound":{"type":"socks","tag":"two","server":"two.example","server_port":1080}}]`)
	second, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: SubscriptionSourceVersion{
			ID: "version-current", SourceID: source.ID, Format: "uri-list",
			RawBody: []byte(`socks://two.example:1080`), NormalizedNodes: nodesV2,
			Diagnostics: json.RawMessage(`[]`), FetchedAt: now.Add(2 * time.Minute), CreatedAt: now.Add(2 * time.Minute),
		},
		ExpectedSourceUpdatedAt: first.Source.UpdatedAt, UpdatedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ActivateSubscriptionSourceVersionAndTask(
		ctx, source.ID, first.Version.ID, second.Source.UpdatedAt, now.Add(3*time.Minute), &refreshTask,
	); err == nil {
		t.Fatal("version restore succeeded despite refresh task identity collision")
	}
	afterRestore, err := database.GetSubscriptionSource(ctx, source.ID)
	if err != nil || afterRestore.CurrentVersionID != second.Version.ID || !afterRestore.UpdatedAt.Equal(second.Source.UpdatedAt) {
		t.Fatalf("failed restore changed source pointer: source=%+v err=%v", afterRestore, err)
	}
}

func TestSubscriptionCurrentVersionMustBelongToSameSource(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 27, 10, 0, 0, 0, time.UTC)
	firstSource, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-owner", Name: "owner", SourceKind: SubscriptionSourceLocal,
		Config: json.RawMessage(`{}`), Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondSource, err := database.CreateSubscriptionSource(ctx, SubscriptionSource{
		ID: "source-other", Name: "other", SourceKind: SubscriptionSourceLocal,
		Config: json.RawMessage(`{}`), Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := database.SaveSubscriptionSourceVersion(ctx, SaveSubscriptionSourceVersionInput{
		Version: SubscriptionSourceVersion{
			ID: "version-owned", SourceID: firstSource.ID, Format: "uri-list",
			RawBody: []byte(`socks://one.example:1080`), NormalizedNodes: json.RawMessage(`[]`),
			Diagnostics: json.RawMessage(`[]`), FetchedAt: now.Add(time.Minute), CreatedAt: now.Add(time.Minute),
		},
		ExpectedSourceUpdatedAt: firstSource.UpdatedAt, UpdatedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(
		ctx, `UPDATE subscription_sources SET current_version_id = ? WHERE id = ?`,
		version.Version.ID, secondSource.ID,
	); err == nil {
		t.Fatal("database accepted a current version owned by another source")
	}
	if _, err := database.db.ExecContext(
		ctx,
		`INSERT INTO subscription_sources(
			id, name, source_kind, config_json, current_version_id, enabled, created_at, updated_at
		 ) VALUES ('source-insert', 'insert', 'local', '{}', ?, 1, ?, ?)`,
		version.Version.ID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	); err == nil {
		t.Fatal("database accepted an initial current version owned by another source")
	}
	reloaded, err := database.GetSubscriptionSource(ctx, secondSource.ID)
	if err != nil || reloaded.CurrentVersionID != "" {
		t.Fatalf("cross-source pointer was persisted: source=%+v err=%v", reloaded, err)
	}
}
