// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func newPoolTestApplication(t *testing.T) (*store.Store, *Service) {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := newSubscriptionTestApplication(db)
	clock := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	return db, app
}

func createFileSource(t *testing.T, app *Service, name, content string) SubscriptionSource {
	t.Helper()
	source, err := app.CreateSubscriptionSource(t.Context(), CreateSubscriptionSourceRequest{
		Name: name, Kind: store.SubscriptionSourceFile, Filter: &SourceFilter{}, Content: &content,
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func replaceContent(t *testing.T, app *Service, sourceID, content string) {
	t.Helper()
	if err := giveContent(t, app, sourceID, content); err != nil {
		t.Fatal(err)
	}
}

// giveContent saves a file source with new content and its settings unchanged.
func giveContent(t *testing.T, app *Service, sourceID, content string) error {
	t.Helper()
	current, err := app.SubscriptionSource(t.Context(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.UpdateSubscriptionSource(t.Context(), sourceID, UpdateSubscriptionSourceRequest{
		Name: current.Name, Format: current.Format, Filter: &current.Filter, Content: &content,
	}, current.UpdatedAt)
	return err
}

// poolSummary is the pool's display names in order, and each node's ID by
// display name.
func poolSummary(t *testing.T, app *Service) ([]string, map[string]SubscriptionNode) {
	t.Helper()
	list, err := app.SubscriptionNodes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	nodes := map[string]SubscriptionNode{}
	for _, node := range list.Items {
		names = append(names, node.DisplayName)
		nodes[node.DisplayName] = node
	}
	return names, nodes
}

func TestPoolSynchronizationKeepsIdentityAndAppendsNewNodes(t *testing.T) {
	ctx := t.Context()
	db, app := newPoolTestApplication(t)
	testutil.PublishNode(t, db, "tokyo", testutil.InboundDocument, "tokyo.example", "")
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	first := createFileSource(t, app, "first", "socks5://hk.example:1080#HK\nsocks5://jp.example:1080#JP\n")
	createFileSource(t, app, "second", "socks5://other-hk.example:1080#hk\nsocks5://sg.example:1080#TOKYO · VLESS-IN\n")
	names, nodes := poolSummary(t, app)
	// Display names stay unique ignoring case.
	if want := []string{"tokyo · vless-in", "HK", "JP", "hk 2", "TOKYO · VLESS-IN 2"}; !slices.Equal(names, want) {
		t.Fatalf("pool = %v, want %v", names, want)
	}
	hk, jp := nodes["HK"], nodes["JP"]
	if hk.ID != poolNodeID(originSource, first.ID, "HK") || len(hk.ID) != 3+poolNodeIDLength || hk.Server != "hk.example" {
		t.Fatalf("source node = %+v", hk)
	}

	// The same content, reordered, keeps every node's ID, name and position;
	// a new node joins the end of the order and new content replaces old.
	renamed, err := app.UpdateSubscriptionNode(ctx, jp.ID, "Japan", jp.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateSubscriptionNode(ctx, jp.ID, "Japan 2", jp.UpdatedAt); !errors.Is(err, store.ErrSubscriptionConflict) {
		t.Fatalf("stale rename error = %v", err)
	}
	if _, err := app.UpdateSubscriptionNode(ctx, jp.ID, "hk", renamed.UpdatedAt); !errors.Is(err, store.ErrSubscriptionNodeNameTaken) {
		t.Fatalf("taken name error = %v", err)
	}
	replaceContent(t, app, first.ID, "socks5://kr.example:1080#KR\nsocks5://jp.example:2080#JP\nsocks5://hk.example:1080#HK\n")
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	names, nodes = poolSummary(t, app)
	if want := []string{"tokyo · vless-in", "HK", "Japan", "hk 2", "TOKYO · VLESS-IN 2", "KR"}; !slices.Equal(names, want) {
		t.Fatalf("pool = %v, want %v", names, want)
	}
	if nodes["HK"].ID != hk.ID || nodes["Japan"].ID != jp.ID || nodes["Japan"].Port != 2080 || !nodes["Japan"].UpdatedAt.Equal(renamed.UpdatedAt) {
		t.Fatalf("synchronization changed identity, name or version: %+v", nodes["Japan"])
	}

	// A saved order holds across synchronizations; an order that does not
	// name exactly the current nodes is refused.
	list, err := app.SubscriptionNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, node := range list.Items {
		ids = append(ids, node.ID)
	}
	slices.Reverse(ids)
	for _, stale := range [][]string{ids[1:], append(slices.Clone(ids), "sn_gone"), append(slices.Clone(ids[1:]), ids[1])} {
		if _, err := app.SetSubscriptionNodeOrder(ctx, stale); !errors.Is(err, store.ErrSubscriptionPoolChanged) {
			t.Fatalf("stale order error = %v", err)
		}
	}
	if _, err := app.SetSubscriptionNodeOrder(ctx, ids); err != nil {
		t.Fatal(err)
	}
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	names, _ = poolSummary(t, app)
	if want := []string{"KR", "TOKYO · VLESS-IN 2", "hk 2", "Japan", "HK", "tokyo · vless-in"}; !slices.Equal(names, want) {
		t.Fatalf("saved order lost: %v", names)
	}
}

func TestPoolRemovalLeavesChannelsAndGrants(t *testing.T) {
	ctx := t.Context()
	_, app := newPoolTestApplication(t)
	source := createFileSource(t, app, "source", "socks5://a.example:1080#A\nsocks5://b.example:1080#B\n")
	_, nodes := poolSummary(t, app)
	a, b := nodes["A"].ID, nodes["B"].ID
	config, _ := json.Marshal(map[string]any{
		"groups": []any{map[string]any{"id": "main", "name": "Main", "enabled": true, "type": "select",
			"members": []any{map[string]any{"kind": "node", "id": a}, map[string]any{"kind": "node", "id": b}},
		}},
		"routes":       []any{map[string]any{"id": "rule", "enabled": true, "rule": map[string]any{"kind": "domain", "value": "example.org"}, "exit": map[string]any{"kind": "node", "id": a}}},
		"default_exit": map[string]any{"kind": "node", "id": a},
	})
	channel, err := app.CreateSubscriptionChannel(ctx, CreateSubscriptionChannelRequest{Name: "channel", Format: store.SubscriptionFormatMihomo, Config: config, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	user, err := app.CreateSubscriptionUser(ctx, CreateSubscriptionUserRequest{Name: "user", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReplaceSubscriptionUserGrants(ctx, user.ID, []string{"sn_unknown"}, user.UpdatedAt); !errors.As(err, new(*store.SubscriptionReferenceError)) {
		t.Fatalf("a grant named a node outside the pool: %v", err)
	}
	if _, err := app.ReplaceSubscriptionUserGrants(ctx, user.ID, []string{a, b}, user.UpdatedAt); err != nil {
		t.Fatal(err)
	}

	replaceContent(t, app, source.ID, "socks5://b.example:1080#B\n")
	stored, err := app.SubscriptionChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	group := stored.Config.Groups[0]
	if len(group.Members) != 1 || group.Members[0].ID != b || stored.Config.Routes[0].Exit != (RouteExit{Kind: "reject"}) ||
		stored.Config.DefaultExit != (RouteExit{Kind: "direct"}) {
		t.Fatalf("channel after removal = %+v", stored.Config)
	}
	grants, err := app.SubscriptionUserGrants(ctx, user.ID)
	if err != nil || !slices.Equal(grants.Grants, []string{b}) {
		t.Fatalf("grants after removal = %v, %v", grants.Grants, err)
	}

	// A node that returns is new: it joins the end of the order and no channel.
	if _, err := app.SetSubscriptionNodeOrder(ctx, []string{b}); err != nil {
		t.Fatal(err)
	}
	replaceContent(t, app, source.ID, "socks5://a.example:1080#A\nsocks5://b.example:1080#B\n")
	names, nodes := poolSummary(t, app)
	if !slices.Equal(names, []string{"B", "A"}) || nodes["A"].ID != a {
		t.Fatalf("pool = %v", names)
	}
	stored, err = app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || len(stored.Config.Groups[0].Members) != 1 {
		t.Fatalf("a returning node rejoined a channel: %+v, %v", stored.Config, err)
	}

	// Deleting the source releases exits that name its nodes the same way.
	config, _ = json.Marshal(map[string]any{
		"groups":       []any{map[string]any{"id": "main", "name": "Main", "enabled": true, "type": "select", "members": []any{}}},
		"routes":       []any{map[string]any{"id": "rule", "enabled": true, "rule": map[string]any{"kind": "domain", "value": "example.org"}, "exit": map[string]any{"kind": "node", "id": b}}},
		"default_exit": map[string]any{"kind": "node", "id": a},
	})
	if _, err := app.UpdateSubscriptionChannel(ctx, channel.ID, UpdateSubscriptionChannelRequest{
		Name: "channel", Format: store.SubscriptionFormatMihomo, Config: config, Enabled: true, ExpectedUpdatedAt: stored.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	current, err := app.SubscriptionSource(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteSubscriptionSource(ctx, source.ID, current.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	stored, err = app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || stored.Config.Routes[0].Exit != (RouteExit{Kind: "reject"}) || stored.Config.DefaultExit != (RouteExit{Kind: "direct"}) {
		t.Fatalf("channel after source deletion = %+v, %v", stored.Config, err)
	}
	if grants, err := app.SubscriptionUserGrants(ctx, user.ID); err != nil || len(grants.Grants) != 0 {
		t.Fatalf("grants after source deletion = %v, %v", grants.Grants, err)
	}
}

func TestPoolKeepsNodesOfContentThatNoLongerReads(t *testing.T) {
	ctx := t.Context()
	db, app := newPoolTestApplication(t)
	broken := createFileSource(t, app, "broken", "socks5://a.example:1080#A\n")
	createFileSource(t, app, "working", "socks5://b.example:1080#B\n")
	// Stored content a later release cannot read.
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE subscription_sources SET content = CAST('not a link' AS BLOB) WHERE id = ?`, broken.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.SyncPool(ctx); err != nil {
		t.Fatal(err)
	}
	names, _ := poolSummary(t, app)
	source, err := app.SubscriptionSource(ctx, broken.ID)
	if err != nil || !slices.Equal(names, []string{"A", "B"}) || source.LastError != SourceErrorInvalidContent || source.NodeCount != 1 {
		t.Fatalf("pool = %v, source = %+v, %v", names, source, err)
	}
	// Its filter can still change, and the nodes stay until content reads.
	if _, err := app.UpdateSubscriptionSource(ctx, broken.ID, UpdateSubscriptionSourceRequest{Name: "broken", Filter: &SourceFilter{Exclude: []string{"A"}}}, source.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if names, _ := poolSummary(t, app); !slices.Equal(names, []string{"A", "B"}) {
		t.Fatalf("pool = %v", names)
	}
}

func TestFileSourceSettingsAndContentSaveTogether(t *testing.T) {
	ctx := t.Context()
	_, app := newPoolTestApplication(t)
	source := createFileSource(t, app, "source", "socks5://a.example:1080#A\n")
	// The new filter reads the new content, not the stored one.
	updated, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{
		Name: "renamed", Filter: &SourceFilter{Include: []string{"B"}},
		Content: new("socks5://a.example:1080#A\nsocks5://b.example:1080#B\n"),
	}, source.UpdatedAt)
	if err != nil || updated.Name != "renamed" || updated.NodeCount != 1 {
		t.Fatalf("updated = %+v, %v", updated, err)
	}
	if names, _ := poolSummary(t, app); !slices.Equal(names, []string{"B"}) {
		t.Fatalf("pool = %v", names)
	}
}

func TestStrictContentFailuresChangeNothing(t *testing.T) {
	ctx := t.Context()
	_, app := newPoolTestApplication(t)
	source := createFileSource(t, app, "source", "socks5://a.example:1080#A\n")
	for _, content := range []string{"socks5://a.example:1080#A\ntrojan://no-password.example:443#Broken\n", "not links at all"} {
		err := giveContent(t, app, source.ID, content)
		var parseErr *SourceParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("content %q error = %v", content, err)
		}
	}
	_, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "rejected", Kind: store.SubscriptionSourceFile, Filter: &SourceFilter{},
		Content: new("trojan://broken.example:443#Broken\n"),
	})
	var parseErr *SourceParseError
	if !errors.As(err, &parseErr) || parseErr.Location != "line 1" || parseErr.Name != "Broken" {
		t.Fatalf("create error = %#v", err)
	}
	if _, err := app.UpdateSubscriptionSource(ctx, source.ID, UpdateSubscriptionSourceRequest{Name: "source", Filter: &SourceFilter{Include: []string{"nothing"}}}, source.UpdatedAt.Add(-time.Second)); !errors.Is(err, store.ErrSubscriptionConflict) {
		t.Fatalf("stale settings update error = %v", err)
	}
	stored, err := app.SubscriptionSource(ctx, source.ID)
	names, _ := poolSummary(t, app)
	sources, listErr := app.ListSubscriptionSources(ctx, SubscriptionListRequest{})
	if err != nil || listErr != nil || !stored.UpdatedAt.Equal(source.UpdatedAt) || stored.ContentAt != source.ContentAt ||
		!slices.Equal(names, []string{"A"}) || len(sources.Items) != 1 {
		t.Fatalf("a failed change saved something: %+v %v %v", stored, names, err)
	}
}

func TestFleetChangesSynchronizeThePool(t *testing.T) {
	ctx := t.Context()
	db, app := newPoolTestApplication(t)
	id := testutil.PublishNode(t, db, "edge", testutil.InboundDocument, "edge.example", "")
	app.PoolInputsChanged(ctx)
	_, nodes := poolSummary(t, app)
	if nodes["edge · vless-in"].Server != "edge.example" {
		t.Fatalf("pool = %+v", nodes)
	}
	node, err := db.Node(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveNodeSettings(ctx, id, node.Settings.Revision, store.NodeSettings{
		PublicHost: "moved.example", Traffic: store.NodeTrafficSettings{PeriodMonths: 1, ResetDay: 1},
	}); err != nil {
		t.Fatal(err)
	}
	app.PoolInputsChanged(context.Background())
	_, moved := poolSummary(t, app)
	if moved["edge · vless-in"].Server != "moved.example" || moved["edge · vless-in"].ID != nodes["edge · vless-in"].ID {
		t.Fatalf("pool = %+v", moved)
	}
	if err := db.DeleteNode(ctx, id); err != nil {
		t.Fatal(err)
	}
	if names, _ := poolSummary(t, app); len(names) != 0 {
		t.Fatalf("a deleted node's inbounds stayed: %v", names)
	}
}
