// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
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
