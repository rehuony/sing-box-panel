// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"database/sql"
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

	catalog, err := app.SubscriptionNodeCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	servers := map[string]string{}
	available := map[string]bool{}
	ids := map[string]bool{}
	for _, node := range catalog.Nodes {
		if node.Origin != "local" {
			continue
		}
		servers[node.NodeID], available[node.NodeID], ids[node.ID] = node.Server, node.Available, true
		if node.SourceName == "" || node.Listener != "[::]:443" {
			t.Fatalf("local node without provenance or listener: %+v", node)
		}
	}
	if servers[override] != "tokyo.example.com" || servers[reported] != "203.0.113.2" || servers[hostless] != "" ||
		!available[override] || !available[reported] || available[hostless] || len(ids) != 3 {
		t.Fatalf("local nodes: servers=%v available=%v ids=%d", servers, available, len(ids))
	}
	if len(catalog.NodePublications) != 3 {
		t.Fatalf("node publications = %+v", catalog.NodePublications)
	}

	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{Name: "public", Format: store.SubscriptionFormatSingBox, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := app.RenderSubscriptionDraft(ctx, "", channel.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := preview.Result.Content
	for _, want := range []string{`"server":"tokyo.example.com"`, `"server":"203.0.113.2"`, `"tokyo · vless-in"`, `"osaka · vless-in"`} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("preview lacks %s: %s", want, content)
		}
	}
	if bytes.Contains(content, []byte("kyoto")) || bytes.Contains(content, []byte(catalogHost)) {
		t.Fatalf("a node without a public host was published: %s", content)
	}
	found := false
	for _, diagnostic := range preview.Result.Diagnostics {
		found = found || diagnostic.OriginNodeID == hostless && diagnostic.Code == DiagnosticPublicHostUnavailable && diagnostic.OriginNodeName == "kyoto"
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want the hostless node named", preview.Result.Diagnostics)
	}

	// A node whose reported digest does not match the panel's revision
	// publishes nothing.
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE nodes SET applied_config_sha256 = ? WHERE id = ?`, strings.Repeat("0", 64), reported)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	preview, err = app.RenderSubscriptionDraft(ctx, "", channel.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(preview.Result.Content, []byte("osaka")) || !bytes.Contains(preview.Result.Content, []byte("tokyo")) {
		t.Fatalf("mismatched node published or the others withheld: %s", preview.Result.Content)
	}
}
