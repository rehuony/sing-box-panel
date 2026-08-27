// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/manualjson"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestPrepareActivationBundleFreezesAllEnabledPublicationInputs(t *testing.T) {
	ctx := context.Background()
	database, app, startup, raw := subscriptionPublicationFixture(t, ctx)

	if _, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "disabled", Format: store.SubscriptionFormatLoon,
		Config: json.RawMessage(`{}`), Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "disabled", SourceKind: store.SubscriptionSourceRemote,
		Config:         json.RawMessage(`{"url":"https://disabled.invalid"}`),
		LatestSnapshot: json.RawMessage(`[{"tag":"disabled"}]`), Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	enabledSource, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "enabled", SourceKind: store.SubscriptionSourceLocal,
		Config:         json.RawMessage(`{"path":"/not-frozen-secret"}`),
		LatestSnapshot: json.RawMessage(`[{"type":"vless","tag":"source-v1","server":"source.example","server_port":443,"uuid":"source-uuid"}]`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 12
	results := make(chan ActivationPreparation, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			prepared, prepareErr := app.PrepareActivationBundle(
				ctx,
				startup.ID,
				store.MonitoringProcessOnly,
			)
			if prepareErr != nil {
				errorsChannel <- prepareErr
				return
			}
			results <- prepared
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)
	for prepareErr := range errorsChannel {
		t.Fatalf("concurrent preparation: %v", prepareErr)
	}

	var prepared ActivationPreparation
	for result := range results {
		if prepared.Bundle.ID == "" {
			prepared = result
			continue
		}
		if result.Bundle.ID != prepared.Bundle.ID || result.Snapshot.ID != prepared.Snapshot.ID ||
			result.Bundle.SHA256 != prepared.Bundle.SHA256 || result.Snapshot.SHA256 != prepared.Snapshot.SHA256 {
			t.Fatalf("non-deterministic preparation: first=%+v next=%+v", prepared, result)
		}
	}
	if prepared.Bundle.ID == "" {
		t.Fatal("no activation preparation returned")
	}

	var wire subscriptionSnapshotWire
	if err := json.Unmarshal(prepared.Snapshot.Content, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.SchemaVersion != publicSubscriptionSnapshotSchema || len(wire.Channels) != 2 {
		t.Fatalf("snapshot wire = %+v", wire)
	}
	formats := map[store.SubscriptionFormat]bool{}
	for _, channel := range wire.Channels {
		formats[channel.Format] = true
		if !bytes.Contains(channel.Body, []byte("publish.example")) ||
			!bytes.Contains(channel.Body, []byte("source.example")) || channel.BodySHA256 == "" {
			t.Fatalf("frozen channel = %+v body=%s", channel, channel.Body)
		}
	}
	if !formats[store.SubscriptionFormatSingBox] || !formats[store.SubscriptionFormatMihomo] ||
		formats[store.SubscriptionFormatLoon] {
		t.Fatalf("frozen formats = %+v", formats)
	}

	var sources []frozenSubscriptionSourceWire
	if err := json.Unmarshal(prepared.Bundle.SourceSnapshots, &sources); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceID != enabledSource.ID ||
		!bytes.Contains(sources[0].Snapshot, []byte(`"tag":"source-v1"`)) ||
		!bytes.Contains(sources[0].Snapshot, []byte(`"server":"source.example"`)) ||
		strings.Contains(string(prepared.Bundle.SourceSnapshots), "not-frozen-secret") {
		t.Fatalf("frozen sources = %s", prepared.Bundle.SourceSnapshots)
	}
	storedStartup, err := database.GetStartupArtifact(ctx, startup.ID)
	if err != nil || !bytes.Equal(storedStartup.ConfigBytes, raw) {
		t.Fatalf("manual startup bytes changed: %q, err=%v", storedStartup.ConfigBytes, err)
	}

	// A stronger tier is rejected until the server can supply matching live
	// probe evidence; it is never accepted as a label over process-only health.
	if _, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringFull); !errors.Is(err, ErrMonitoringTierUnavailable) {
		t.Fatalf("full monitoring tier error = %v", err)
	}
}

func TestPublicSubscriptionUsesAppliedFrozenSnapshotAndImmediateTokenState(t *testing.T) {
	ctx := context.Background()
	database, app, startup, _ := subscriptionPublicationFixture(t, ctx)
	channels, err := app.ListSubscriptionChannels(ctx, SubscriptionListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var singBoxSummary SubscriptionChannelSummary
	for _, channel := range channels.Items {
		if channel.Format == store.SubscriptionFormatSingBox {
			singBoxSummary = channel
		}
	}
	if singBoxSummary.ID == "" {
		t.Fatal("missing sing-box channel fixture")
	}
	singBox, err := app.SubscriptionChannel(ctx, singBoxSummary.ID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "mutable-source", SourceKind: store.SubscriptionSourceLocal,
		Config: json.RawMessage(`{}`), LatestSnapshot: json.RawMessage(`[{"type":"vless","tag":"source-v1","server":"source-v1.example","server_port":443,"uuid":"source-v1"}]`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	applyActivationBundle(t, ctx, database, app, first.Bundle.ID)

	created, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := app.PublicSubscription(ctx, created.Token, singBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Format != store.SubscriptionFormatSingBox || before.NodeCount != 2 ||
		!bytes.Contains(before.Body, []byte("publish.example")) ||
		!bytes.Contains(before.Body, []byte("source-v1.example")) {
		t.Fatalf("public subscription = %+v body=%s", before, before.Body)
	}
	if _, err := app.PublicSubscription(ctx, created.Token, "channel-missing"); !errors.Is(err, ErrPublicSubscriptionChannelUnavailable) {
		t.Fatalf("missing channel error = %v", err)
	}

	// Mutable control-plane changes can prepare a new bundle, but cannot alter
	// publication until that exact bundle becomes applied.
	singBox, err = app.UpdateSubscriptionChannel(ctx, singBox.ID, UpdateSubscriptionChannelRequest{
		Name: singBox.Name, Format: singBox.Format,
		Config: json.RawMessage(`{"exclude_tags":["publish"]}`), Enabled: true,
		ExpectedUpdatedAt: singBox.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateSubscriptionSourceSnapshot(ctx, source.ID, UpdateSubscriptionSourceSnapshotRequest{
		LatestSnapshot: json.RawMessage(`[{"type":"vless","tag":"source-v2","server":"source-v2.example","server_port":443,"uuid":"source-v2"}]`), ExpectedUpdatedAt: source.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	second, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if second.Bundle.ID == first.Bundle.ID || second.Snapshot.ID == first.Snapshot.ID {
		t.Fatalf("mutable inputs did not produce a new frozen identity: first=%s second=%s", first.Bundle.ID, second.Bundle.ID)
	}
	stillFirst, err := app.PublicSubscription(ctx, created.Token, singBox.ID)
	if err != nil || !bytes.Equal(stillFirst.Body, before.Body) {
		t.Fatalf("unapplied mutation changed publication: body=%s err=%v", stillFirst.Body, err)
	}
	applyActivationBundle(t, ctx, database, app, second.Bundle.ID)
	afterApply, err := app.PublicSubscription(ctx, created.Token, singBox.ID)
	if err != nil || bytes.Equal(afterApply.Body, before.Body) || afterApply.NodeCount != 1 ||
		bytes.Contains(afterApply.Body, []byte("publish.example")) ||
		bytes.Contains(afterApply.Body, []byte("source-v1.example")) ||
		!bytes.Contains(afterApply.Body, []byte("source-v2.example")) {
		t.Fatalf("applied publication did not switch atomically: %+v body=%s err=%v", afterApply, afterApply.Body, err)
	}

	rotation, err := app.RotateSubscriptionToken(ctx, created.Metadata.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.PublicSubscription(ctx, created.Token, singBox.ID); !errors.Is(err, ErrPublicSubscriptionAccessDenied) || strings.Contains(err.Error(), created.Token) {
		t.Fatalf("rotated plaintext error = %v", err)
	}
	if _, err := app.PublicSubscription(ctx, rotation.Token, singBox.ID); err != nil {
		t.Fatalf("replacement token: %v", err)
	}
	if _, err := app.RevokeSubscriptionToken(ctx, rotation.Created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PublicSubscription(ctx, rotation.Token, singBox.ID); !errors.Is(err, ErrPublicSubscriptionAccessDenied) || strings.Contains(err.Error(), rotation.Token) {
		t.Fatalf("revoked plaintext error = %v", err)
	}
	if _, err := app.PublicSubscription(ctx, "unknown-token", singBox.ID); !errors.Is(err, ErrPublicSubscriptionAccessDenied) {
		t.Fatalf("unknown token error = %v", err)
	}
}

func TestPublicSubscriptionRequiresAppliedBundleAndRejectsExpiredToken(t *testing.T) {
	ctx := context.Background()
	_, app, _, _ := subscriptionPublicationFixture(t, ctx)
	channels, err := app.ListSubscriptionChannels(ctx, SubscriptionListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.PublicSubscription(ctx, token.Token, channels.Items[0].ID); !errors.Is(err, store.ErrNoAppliedBundle) {
		t.Fatalf("no applied bundle error = %v", err)
	}

	now := app.now().UTC()
	expiresAt := now.Add(time.Second)
	expiring, err := app.CreateSubscriptionToken(ctx, CreateSubscriptionTokenRequest{
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.now = func() time.Time { return expiresAt }
	if _, err := app.PublicSubscription(ctx, expiring.Token, channels.Items[0].ID); !errors.Is(err, ErrPublicSubscriptionAccessDenied) || strings.Contains(err.Error(), expiring.Token) {
		t.Fatalf("expired token error = %v", err)
	}
}

func TestActivationPreparationRejectsAmbiguousManualJSONBeforePersistence(t *testing.T) {
	ctx := context.Background()
	database, app, startup, _ := subscriptionPublicationFixture(t, ctx)
	duplicate, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-duplicate-jsonc", Kind: store.StartupArtifactManual,
		CanonicalRevisionID: startup.CanonicalRevisionID,
		ExactCoreVersion:    startup.ExactCoreVersion, RendererVersion: "manual-v1",
		CoreArtifactID: startup.CoreArtifactID,
		ConfigBytes:    []byte(`{"outbounds":[],"outbounds":[]}`),
		CreatedAt:      startup.CreatedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err = database.CompleteStartupArtifactCheck(
		ctx,
		duplicate.ID,
		true,
		nil,
		startup.CreatedAt.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.PrepareActivationBundle(ctx, duplicate.ID, store.MonitoringProcessOnly); !errors.Is(err, manualjson.ErrInvalidManualJSON) {
		t.Fatalf("duplicate JSONC error = %v", err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Hub.DesiredBundleID != "" || bootstrap.Hub.AppliedBundleID != "" {
		t.Fatalf("failed preparation changed hub: %+v", bootstrap.Hub)
	}
}

func subscriptionPublicationFixture(
	t *testing.T,
	ctx context.Context,
) (*store.Store, *Application, store.StartupArtifact, []byte) {
	t.Helper()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)
	now := app.now().UTC()
	core := applicationTestCore("core-publication", "1.13.19", 'c', 'd', now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	revision, err := app.ReplaceCanonical(ctx, "", canonical.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  // exact manual bytes stay attached to the runtime artifact
  "outbounds": [
    {"type":"shadowsocks","tag":"publish","server":"publish.example","server_port":443,"method":"aes-256-gcm","password":"secret"},
  ],
}
`)
	startup, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-publication", Kind: store.StartupArtifactManual,
		CanonicalRevisionID: revision.Revision.ID, ExactCoreVersion: core.ExactVersion,
		RendererVersion: "manual-v1", CoreArtifactID: core.ID,
		ConfigBytes: raw, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, nil, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "sing-box", Format: store.SubscriptionFormatSingBox,
		Config: json.RawMessage(`{}`), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{
		Name: "mihomo", Format: store.SubscriptionFormatMihomo,
		Config: json.RawMessage(`{}`), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	return database, app, startup, raw
}

func applyActivationBundle(
	t *testing.T,
	ctx context.Context,
	database *store.Store,
	app *Application,
	bundleID string,
) {
	t.Helper()
	queued, err := app.QueueRuntimeApply(ctx, bundleID)
	if err != nil {
		t.Fatal(err)
	}
	now := app.now().UTC().Add(10 * time.Minute)
	claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
		Lane: store.TaskLaneRuntime, LeaseOwner: "subscription-publication-test",
		Now: now, LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("claim apply task = %+v, err=%v", claimed, err)
	}
	if _, err := database.CompleteTask(
		ctx,
		claimed.ID,
		claimed.LeaseOwner,
		now.Add(time.Second),
		store.TaskCompletion{Succeeded: true, Result: json.RawMessage(`{"healthy":true}`)},
	); err != nil {
		t.Fatal(err)
	}
}
