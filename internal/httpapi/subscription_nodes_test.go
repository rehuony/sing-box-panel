package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func TestManualNodesCRUDVisibilityAndPublicationWithoutCore(t *testing.T) {
	ctx := context.Background()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	raw := `{"type":"socks","tag":"香港","server":"manual.example","server_port":1080,"future":{"large":9007199254740993},"password":"not-in-summary"}`
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/subscription/nodes", strings.NewReader(`{"outbound":`+raw+`}`)))
	if unauthenticated.Code != 401 {
		t.Fatalf("unauthenticated %d", unauthenticated.Code)
	}
	created := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/nodes", `{"outbound":`+raw+`}`, "")
	if created.Code != 201 {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	var node application.SubscriptionNodeDetail
	if err := json.Unmarshal(created.Body.Bytes(), &node); err != nil {
		t.Fatal(err)
	}
	if node.Name != "香港" || node.SourceName != "手动节点" || node.Revision != 1 || !strings.Contains(node.OutboundJSON, "9007199254740993") {
		t.Fatalf("wrong detail %+v", node)
	}
	catalog := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/nodes", "", "")
	if catalog.Code != 200 || strings.Contains(catalog.Body.String(), "not-in-summary") {
		t.Fatalf("catalog %d %s", catalog.Code, catalog.Body.String())
	}
	channel, err := app.CreateSubscriptionChannel(ctx, application.CreateSubscriptionChannelRequest{Name: "channel", Format: store.SubscriptionFormatSingBox, PublicHost: "public.example", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "channel key"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/sub/" + key.Token + "/" + channel.ID
	response := publicSubscriptionRequest(handler, path)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "manual.example") {
		t.Fatalf("publish %d %s", response.Code, response.Body.String())
	}
	endpoint := "/api/v1/subscription/nodes/" + node.ID
	hidden := authenticatedRequest(t, handler, http.MethodPut, endpoint+"/visibility", `{"hidden":true,"revision":0}`, "")
	if hidden.Code != 200 {
		t.Fatalf("hide %d %s", hidden.Code, hidden.Body.String())
	}
	stale := authenticatedRequest(t, handler, http.MethodPut, endpoint+"/visibility", `{"hidden":false,"revision":0}`, "")
	if stale.Code != 412 {
		t.Fatalf("stale hide %d", stale.Code)
	}
	response = publicSubscriptionRequest(handler, path)
	if response.Code != 200 || strings.Contains(response.Body.String(), "manual.example") {
		t.Fatalf("hidden published %d %s", response.Code, response.Body.String())
	}
	newRaw := strings.ReplaceAll(raw, "manual.example", "changed.example")
	updated := authenticatedRequest(t, handler, http.MethodPut, endpoint, `{"revision":1,"outbound":`+newRaw+`}`, "")
	if updated.Code != 200 || !strings.Contains(updated.Body.String(), `"hidden":true`) {
		t.Fatalf("update %d %s", updated.Code, updated.Body.String())
	}
	stale = authenticatedRequest(t, handler, http.MethodDelete, endpoint, `{"revision":1}`, "")
	if stale.Code != 412 {
		t.Fatalf("stale delete %d", stale.Code)
	}
	restored := authenticatedRequest(t, handler, http.MethodPut, endpoint+"/visibility", `{"hidden":false,"revision":1}`, "")
	if restored.Code != 200 {
		t.Fatalf("restore %d", restored.Code)
	}
	response = publicSubscriptionRequest(handler, path)
	if !strings.Contains(response.Body.String(), "changed.example") {
		t.Fatalf("restored missing %s", response.Body.String())
	}
	deleted := authenticatedRequest(t, handler, http.MethodDelete, endpoint, `{"revision":2}`, "")
	if deleted.Code != 204 {
		t.Fatalf("delete %d %s", deleted.Code, deleted.Body.String())
	}
	missing := authenticatedRequest(t, handler, http.MethodGet, endpoint, "", "")
	if missing.Code != 404 {
		t.Fatalf("after delete %d", missing.Code)
	}
}

func TestSourceNodeVisibilitySurvivesRefreshAndDoesNotBroadenLegacyGrants(t *testing.T) {
	ctx := context.Background()
	db, app, handler, channel, startup := newSubscriptionPublicationHTTPFixture(t, "")
	prepared, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	applySubscriptionHTTPBundle(t, db, app, prepared.Bundle.ID)
	source, err := app.CreateSubscriptionSource(ctx, application.CreateSubscriptionSourceRequest{Name: "Global Edge", SourceKind: store.SubscriptionSourceLocal, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	version, err := app.CreateSubscriptionSourceVersion(ctx, source.ID, application.CreateSubscriptionSourceVersionRequest{Format: "sing-box-json", RawBody: []byte(`{"type":"socks","tag":"東京","server":"first.example","server_port":1080}`), ExpectedUpdatedAt: source.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := app.SubscriptionNodeCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	remote := catalog.Nodes[len(catalog.Nodes)-1]
	local := catalog.Nodes[0]
	if remote.SourceName != "手动节点" || remote.SourceID != source.ID || remote.Name != "東京" || local.Origin != "local" {
		t.Fatalf("provenance %+v", catalog.Nodes)
	}
	if _, err := app.SetSubscriptionNodeVisibility(ctx, remote.ID, true, 0); err != nil {
		t.Fatal(err)
	}
	user, _ := app.CreateSubscriptionUser(ctx, application.CreateSubscriptionUserRequest{Name: "legacy", Enabled: true})
	if _, err := app.ReplaceSubscriptionUserGrants(ctx, user.ID, []string{remote.Key}, user.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	key, _ := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{UserID: user.ID, Label: "legacy"})
	_, err = app.CreateSubscriptionSourceVersion(ctx, source.ID, application.CreateSubscriptionSourceVersionRequest{Format: "sing-box-json", RawBody: []byte(`{"type":"socks","tag":"東京","server":"second.example","server_port":1080}`), ExpectedUpdatedAt: version.Source.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	refreshed, _ := app.SubscriptionNodeCatalog(ctx)
	after := refreshed.Nodes[len(refreshed.Nodes)-1]
	if after.ID != remote.ID || !after.Hidden || after.Key == remote.Key {
		t.Fatalf("identities before=%+v after=%+v", remote, after)
	}
	if _, err := app.SetSubscriptionNodeVisibility(ctx, after.ID, false, 1); err != nil {
		t.Fatal(err)
	}
	response := publicSubscriptionRequest(handler, "/sub/"+key.Token+"/"+channel.ID)
	if response.Code != 200 || strings.Contains(response.Body.String(), "second.example") {
		t.Fatalf("grant widened %d %s", response.Code, response.Body.String())
	}
	denied := authenticatedRequest(t, handler, http.MethodDelete, "/api/v1/subscription/nodes/"+local.ID, `{"revision":1}`, "")
	if denied.Code != 422 {
		t.Fatalf("local deletion %d %s", denied.Code, denied.Body.String())
	}
	if err := app.DeleteSubscriptionNode(ctx, after.ID, 1); err != store.ErrSubscriptionNodeInvalid {
		t.Fatalf("external mutation %v", err)
	}
}

func TestParseSingleNodeDoesNotSaveAndRejectsMultiple(t *testing.T) {
	_, app, handler := newSubscriptionHTTPServices(t, "")
	raw := `{"type":"socks","tag":"single","server":"example.com","server_port":1080}`
	body, _ := json.Marshal(map[string]string{"text": raw})
	parsed := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/nodes/parse", string(body), "")
	if parsed.Code != 200 || parsed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("parse %d %s", parsed.Code, parsed.Body.String())
	}
	catalog, _ := app.SubscriptionNodeCatalog(context.Background())
	if len(catalog.Nodes) != 0 {
		t.Fatal("parse saved a node")
	}
	for _, input := range []string{`[]`, fmt.Sprintf(`[%s,%s]`, raw, strings.Replace(raw, "single", "second", 1)), `{"type":"socks","tag":"missing"}`} {
		body, _ = json.Marshal(map[string]string{"text": input})
		result := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/nodes/parse", string(body), "")
		if result.Code != 422 {
			t.Fatalf("invalid parse %d %s", result.Code, result.Body.String())
		}
	}
}

func TestNodeOrderPreviewDeliveryAndConflict(t *testing.T) {
	ctx := t.Context()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	nodes := make([]application.SubscriptionNodeDetail, 0, 2)
	for _, name := range []string{"Alpha", "Beta"} {
		node, err := app.CreateSubscriptionNode(ctx, []byte(fmt.Sprintf(`{"type":"socks","tag":%q,"server":%q,"server_port":1080}`, name, name+".example")))
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, node)
	}
	key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "order"})
	if err != nil {
		t.Fatal(err)
	}
	type channelCheck struct{ id, path, etag string }
	checks := []channelCheck{}
	for _, format := range []store.SubscriptionFormat{store.SubscriptionFormatSingBox, store.SubscriptionFormatMihomo, store.SubscriptionFormatLoon} {
		for _, modern := range []bool{false, true} {
			config := json.RawMessage(`{}`)
			if modern {
				config, err = json.Marshal(store.SubscriptionChannelConfig{Policy: &subscription.ChannelPolicy{
					Selection:   subscription.NodeSelection{IDs: []string{nodes[0].ID, nodes[1].ID}, ExcludedIDs: []string{}, NewNodePolicy: "exclude"},
					DefaultExit: subscription.RouteExit{Kind: "group", ID: "main"},
					Groups:      []subscription.RuleGroup{{ID: "main", Name: "Selected", Enabled: true, Type: "select", NodeIDs: []string{nodes[0].ID, nodes[1].ID}, BuiltinNodes: []string{}, Rules: []subscription.ChannelRule{}}},
				}})
				if err != nil {
					t.Fatal(err)
				}
			}
			channel, err := app.CreateSubscriptionChannel(ctx, application.CreateSubscriptionChannelRequest{Name: fmt.Sprintf("%s-%t", format, modern), Format: format, Enabled: true, Config: config})
			if err != nil {
				t.Fatal(err)
			}
			path := "/sub/" + key.Token + "/" + channel.ID
			before := publicSubscriptionRequest(handler, path)
			if before.Code != 200 || strings.Index(before.Body.String(), "Alpha.example") >= strings.Index(before.Body.String(), "Beta.example") {
				t.Fatalf("initial order: %d %s", before.Code, before.Body.String())
			}
			checks = append(checks, channelCheck{channel.ID, path, before.Header().Get("ETag")})
		}
	}
	endpoint := "/api/v1/subscription/node-orders/manual"
	body := fmt.Sprintf(`{"ids":[%q,%q],"revision":0}`, nodes[1].ID, nodes[0].ID)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPut, endpoint, strings.NewReader(body)))
	if unauthenticated.Code != 401 {
		t.Fatalf("unauthenticated reorder: %d", unauthenticated.Code)
	}
	saved := authenticatedRequest(t, handler, http.MethodPut, endpoint, body, "")
	if saved.Code != 200 {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	stale := authenticatedRequest(t, handler, http.MethodPut, endpoint, body, "")
	if stale.Code != 412 {
		t.Fatalf("stale save: %d %s", stale.Code, stale.Body.String())
	}
	var conflict Problem
	if err := json.Unmarshal(stale.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conflict.Detail, "node_orders[collectionId].revision") || strings.Contains(conflict.Detail, "ETag") {
		t.Fatalf("incorrect conflict recovery: %s", conflict.Detail)
	}
	for _, invalid := range []string{
		`{"ids":[],"revision":-1}`, `{"ids":[],"revision":9007199254740991}`, `{"ids":[],"revision":null}`, `{"ids":null,"revision":1}`,
		`{"ids":["node_unknown"],"revision":1}`, fmt.Sprintf(`{"ids":[%q,%q],"revision":1}`, nodes[0].ID, nodes[0].ID),
	} {
		response := authenticatedRequest(t, handler, http.MethodPut, endpoint, invalid, "")
		if response.Code != 422 {
			t.Fatalf("invalid order %s: %d %s", invalid, response.Code, response.Body.String())
		}
	}
	catalog := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/nodes", "", "")
	var value application.SubscriptionNodeCatalog
	if err := json.Unmarshal(catalog.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Nodes) != 2 || value.Nodes[0].ID != nodes[1].ID || value.NodeOrders["manual"].Revision != 1 {
		t.Fatalf("catalog order: %+v", value)
	}
	for _, check := range checks {
		delivered := publicSubscriptionRequest(handler, check.path)
		content := delivered.Body.String()
		if delivered.Code != 200 || strings.Index(content, "Beta.example") < 0 || strings.Index(content, "Beta.example") >= strings.Index(content, "Alpha.example") {
			t.Fatalf("reordered delivery: %d %s", delivered.Code, content)
		}
		if delivered.Header().Get("ETag") == check.etag {
			t.Fatal("reorder did not change ETag")
		}
		preview := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+check.id+"/preview", "{}", "")
		var rendered application.SubscriptionPreview
		if preview.Code != 200 {
			t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
		}
		if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
			t.Fatal(err)
		}
		if string(rendered.Result.Content) != content {
			t.Fatal("preview and delivery orders differ")
		}
	}
}
