// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestEachNodePublishesItsAppliedInboundsAtItsOwnHost(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := newSubscriptionTestApplication(db)

	// Every node runs the same inbound tag and credentials.
	publish := func(name, publicHost, reportedIP string) string {
		return testutil.PublishNode(t, db, name, testutil.InboundDocument, publicHost, reportedIP)
	}
	override := publish("tokyo", "tokyo.example.com", "203.0.113.1")
	reported := publish("osaka", "", "203.0.113.2")
	hostless := publish("kyoto", "", "")
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}

	list, err := app.SubscriptionNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	servers := map[string]string{}
	names := []string{}
	for _, node := range list.Items {
		if node.Origin.Kind != "inbound" || node.Origin.InboundTag != "vless-in" || node.OriginName != "vless-in" || node.Protocol != "vless" {
			t.Fatalf("inbound node without provenance: %+v", node)
		}
		servers[node.Origin.NodeID] = node.Server
		names = append(names, node.DisplayName)
	}
	// Fleet nodes contribute in name order.
	if len(list.Items) != 2 || servers[override] != "tokyo.example.com" || servers[reported] != "203.0.113.2" ||
		strings.Join(names, ",") != "osaka · vless-in,tokyo · vless-in" {
		t.Fatalf("pool = %+v", list.Items)
	}
	if len(list.NodePublications) != 3 {
		t.Fatalf("node publications = %+v", list.NodePublications)
	}
	found := false
	for _, diagnostic := range list.Diagnostics {
		found = found || diagnostic.NodeID == hostless && diagnostic.Code == DiagnosticPublicHostUnavailable && diagnostic.ItemIndex == -1
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want the hostless node named", list.Diagnostics)
	}

	config, _ := json.Marshal(map[string]any{"groups": []any{map[string]any{
		"id": "main", "name": "Main", "enabled": true, "type": "select",
		"members": []any{map[string]any{"kind": "node", "id": list.Items[0].ID}, map[string]any{"kind": "node", "id": list.Items[1].ID}},
	}}, "routes": []any{}, "default_exit": map[string]any{"kind": "group", "id": "main"}})
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{Name: "public", Format: store.SubscriptionFormatSingBox, Config: config, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := app.RenderSubscriptionDraft(ctx, "", channel.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := preview.Result.Content
	for _, want := range []string{`"server": "tokyo.example.com"`, `"server": "203.0.113.2"`, `"tag": "tokyo · vless-in"`, `"tag": "osaka · vless-in"`} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("preview lacks %s: %s", want, content)
		}
	}
	if bytes.Contains(content, []byte("kyoto")) {
		t.Fatalf("a node without a public host was published: %s", content)
	}
	found = false
	for _, diagnostic := range preview.Result.Diagnostics {
		found = found || diagnostic.OriginNodeID == hostless && diagnostic.Code == DiagnosticPublicHostUnavailable && diagnostic.OriginNodeName == "kyoto"
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want the hostless node named", preview.Result.Diagnostics)
	}

	// A node whose reported digest does not match the panel's revision
	// leaves the pool, and with it the channel.
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE nodes SET applied_config_sha256 = ? WHERE id = ?`, strings.Repeat("0", 64), reported)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	preview, err = app.RenderSubscriptionDraft(ctx, "", channel.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(preview.Result.Content, []byte("osaka")) || !bytes.Contains(preview.Result.Content, []byte("tokyo")) {
		t.Fatalf("mismatched node published or the others withheld: %s", preview.Result.Content)
	}
	stored, err := app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || len(stored.Config.Groups[0].Members) != 1 || stored.Config.Groups[0].Members[0].ID != list.Items[1].ID {
		t.Fatalf("the removed node stayed a member: %+v, %v", stored.Config, err)
	}
}
