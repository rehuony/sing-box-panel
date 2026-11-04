// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestRenderSubscriptionPreviewUsesAppliedVersionAndSelectedUserGrants(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := app.now().UTC()
	features := json.RawMessage(`{"status":"reported","features":["badlinkname","tfogo_checklinkname0","with_acme","with_ccm","with_clash_api","with_dhcp","with_gvisor","with_naive_outbound","with_ocm","with_purego","with_quic","with_tailscale","with_utls","with_wireguard"]}`)
	core := store.CoreArtifact{
		ID: "core-subscription", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: features, CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	startupBytes := []byte(`{
      "inbounds":[
        {"type":"shadowsocks","tag":"hidden","listen_port":443,"method":"aes-128-gcm","password":"hidden-secret"},
        {"type":"shadowsocks","tag":"public","listen_port":8443,"method":"aes-256-gcm","password":"public-secret"}
      ]
    }`)
	canonicalSave, err := app.configs.SaveConfigurationFile(ctx, nodeconfig.ConfigurationFileWrite{Content: string(startupBytes)})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID:                  "startup-subscription-ready",
		CanonicalRevisionID: canonicalSave.CanonicalRevisionID, ExactCoreVersion: core.ExactVersion,
		CoreArtifactID: core.ID, ConfigBytes: startupBytes,
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err = database.CompleteStartupArtifactCheck(ctx, ready.ID, true, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	channel, err := app.subscriptions().CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{
		Name: "filtered", Format: store.SubscriptionFormatSingBox, PublicHost: "filtered.example",
		Config: json.RawMessage(`{"exclude_tags":["hidden"]}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := app.PrepareActivationBundle(ctx, ready.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	testutil.ApplyBundle(t, database, prepared.Bundle.ID)
	user, err := app.subscriptions().CreateSubscriptionUser(ctx, subscription.CreateSubscriptionUserRequest{Name: "preview", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := app.subscriptions().SubscriptionNodeCatalog(ctx)
	if err != nil || len(catalog.Nodes) != 2 {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	var publicKey string
	for _, node := range catalog.Nodes {
		if node.Tag == "public" {
			publicKey = node.Key
		}
	}
	if _, err := app.subscriptions().ReplaceSubscriptionUserGrants(ctx, user.ID, []string{publicKey}, user.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	preview, err := app.subscriptions().RenderSubscriptionPreview(ctx, user.ID, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.AppliedBundleID != prepared.Bundle.ID || preview.UserID != user.ID ||
		preview.ArtifactState != store.StartupArtifactReady || preview.Result.NodeCount != 1 ||
		!bytes.Contains(preview.Result.Content, []byte(`"tag":"public"`)) ||
		bytes.Contains(preview.Result.Content, []byte(`"tag":"hidden"`)) {
		t.Fatalf("ready preview = %+v, content=%s", preview, preview.Result.Content)
	}
}
