// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

func TestSubscriptionNodePoolHTTP(t *testing.T) {
	_, services, handler := newSubscriptionHTTPServices(t, "")
	ids := createSourceNodes(t, services, "source", "ss://YWVzLTI1Ni1nY206c2VjcmV0@a.example:8388#A\nsocks5://b.example:1080#B\n")

	listed := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/nodes", "", "")
	var list subscription.SubscriptionNodeList
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &list) != nil || len(list.Items) != 2 ||
		list.Items[0].ID != ids[0] || list.Items[0].Origin.Kind != "source" || strings.Contains(listed.Body.String(), "secret") {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	detail := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/nodes/"+ids[0], "", "")
	if detail.Code != http.StatusOK || detail.Header().Get("Cache-Control") != "no-store" || !strings.Contains(detail.Body.String(), `"password":"secret"`) {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	if missing := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/nodes/sn_missing", "", ""); missing.Code != http.StatusNotFound {
		t.Fatalf("missing node status=%d", missing.Code)
	}

	// The display name changes under its updated_at version.
	path := "/api/v1/subscription/nodes/" + ids[0]
	if response := authenticatedRequest(t, handler, http.MethodPut, path, `{"display_name":"Alpha"}`, ""); response.Code != http.StatusPreconditionRequired {
		t.Fatalf("rename without If-Match status=%d body=%s", response.Code, response.Body.String())
	}
	version := detail.Header().Get("ETag")
	renamed := authenticatedRequest(t, handler, http.MethodPut, path, `{"display_name":"Alpha"}`, version)
	if renamed.Code != http.StatusOK || renamed.Header().Get("ETag") == version || !strings.Contains(renamed.Body.String(), `"display_name":"Alpha"`) {
		t.Fatalf("rename status=%d body=%s", renamed.Code, renamed.Body.String())
	}
	if stale := authenticatedRequest(t, handler, http.MethodPut, path, `{"display_name":"Beta"}`, version); stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale rename status=%d body=%s", stale.Code, stale.Body.String())
	}
	current := renamed.Header().Get("ETag")
	if taken := authenticatedRequest(t, handler, http.MethodPut, path, `{"display_name":"b"}`, current); taken.Code != http.StatusConflict ||
		!strings.Contains(taken.Body.String(), "subscription_node_name_taken") {
		t.Fatalf("taken name status=%d body=%s", taken.Code, taken.Body.String())
	}
	for _, name := range []string{`""`, `" padded"`, `"a,b"`, `"` + strings.Repeat("x", 257) + `"`} {
		if invalid := authenticatedRequest(t, handler, http.MethodPut, path, `{"display_name":`+name+`}`, current); invalid.Code != http.StatusUnprocessableEntity {
			t.Fatalf("name %s status=%d body=%s", name, invalid.Code, invalid.Body.String())
		}
	}

	// The order names every node once, or the pool changed since it was read.
	for _, body := range []string{`{"ids":["` + ids[1] + `"]}`, `{"ids":["` + ids[1] + `","` + ids[0] + `","sn_other"]}`} {
		if conflict := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/node-order", body, ""); conflict.Code != http.StatusConflict ||
			!strings.Contains(conflict.Body.String(), "subscription_pool_changed") {
			t.Fatalf("stale order status=%d body=%s", conflict.Code, conflict.Body.String())
		}
	}
	ordered := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/node-order", `{"ids":["`+ids[1]+`","`+ids[0]+`"]}`, "")
	if ordered.Code != http.StatusOK || json.Unmarshal(ordered.Body.Bytes(), &list) != nil || list.Items[0].ID != ids[1] || list.Items[1].DisplayName != "Alpha" {
		t.Fatalf("order status=%d body=%s", ordered.Code, ordered.Body.String())
	}
}

func TestSubscriptionSourceHTTP(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	created := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/sources",
		`{"name":"Upload","kind":"file","filter":{"include":[],"exclude":[]},"content":"socks5://a.example:1080#A\nsocks5://b.example:1080#B"}`, "")
	var source subscription.SubscriptionSource
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &source) != nil || source.NodeCount != 2 || created.Header().Get("ETag") == "" {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	sourcePath := "/api/v1/subscription/sources/" + source.ID
	version := created.Header().Get("ETag")

	// Strict parsing names the node that does not convert and saves nothing.
	rejected := authenticatedRequest(t, handler, http.MethodPut, sourcePath+"/content", `{"content":"socks5://a.example:1080#A\ntrojan://t.example:443#Broken"}`, "")
	var problem Problem
	if rejected.Code != http.StatusUnprocessableEntity || json.Unmarshal(rejected.Body.Bytes(), &problem) != nil ||
		problem.Code != "subscription_source_unreadable" || problem.Fields["location"] != "line 2" || problem.Fields["name"] != "Broken" || problem.Fields["code"] != "node_not_convertible" {
		t.Fatalf("rejected content status=%d body=%s", rejected.Code, rejected.Body.String())
	}
	preview := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/sources/preview",
		`{"content":"socks5://a.example:1080#A\ntrojan://t.example:443#Broken","filter":{"include":[],"exclude":[]}}`, "")
	var reading subscription.SubscriptionSourcePreview
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &reading) != nil || len(reading.Items) != 2 ||
		reading.Error == nil || reading.Error.Location != "line 2" || reading.Format != subscription.SourceFormatShareLinks {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	if ambiguous := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/sources/preview",
		`{"source_id":"`+source.ID+`","content":"x","filter":{"include":[],"exclude":[]}}`, ""); ambiguous.Code != http.StatusUnprocessableEntity {
		t.Fatalf("preview of two inputs status=%d body=%s", ambiguous.Code, ambiguous.Body.String())
	}

	// Settings change under If-Match; content uploads do not change the version.
	settings := `{"name":"Upload","filter":{"include":[],"exclude":["/^B$/"]}}`
	if response := authenticatedRequest(t, handler, http.MethodPut, sourcePath, settings, ""); response.Code != http.StatusPreconditionRequired {
		t.Fatalf("update without If-Match status=%d", response.Code)
	}
	if response := authenticatedRequest(t, handler, http.MethodPut, sourcePath, `{"name":"Upload","filter":{"include":["/(/"],"exclude":[]}}`, version); response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "filter.include[0]") {
		t.Fatalf("invalid filter status=%d body=%s", response.Code, response.Body.String())
	}
	updated := authenticatedRequest(t, handler, http.MethodPut, sourcePath, settings, version)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"node_count":1`) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	if stale := authenticatedRequest(t, handler, http.MethodPut, sourcePath, settings, version); stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale update status=%d", stale.Code)
	}
	replaced := authenticatedRequest(t, handler, http.MethodPut, sourcePath+"/content", `{"content":"socks5://c.example:1080#C"}`, "")
	if replaced.Code != http.StatusOK || replaced.Header().Get("ETag") != updated.Header().Get("ETag") || !strings.Contains(replaced.Body.String(), `"node_count":1`) {
		t.Fatalf("content status=%d body=%s", replaced.Code, replaced.Body.String())
	}
	if refresh := authenticatedRequest(t, handler, http.MethodPost, sourcePath+"/refresh", "", ""); refresh.Code != http.StatusUnprocessableEntity {
		t.Fatalf("file source refresh status=%d", refresh.Code)
	}
	if denied := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/sources",
		`{"name":"Private","kind":"url","url":"http://127.0.0.1/sub","refresh_minutes":0,"filter":{"include":[],"exclude":[]}}`, ""); denied.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(denied.Body.String(), "subscription_source_url_rejected") {
		t.Fatalf("private URL status=%d body=%s", denied.Code, denied.Body.String())
	}
	if deleted := authenticatedRequest(t, handler, http.MethodDelete, sourcePath, "", replaced.Header().Get("ETag")); deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	listed := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/nodes", "", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"items":[]`) {
		t.Fatalf("pool after delete = %s", listed.Body.String())
	}
}
