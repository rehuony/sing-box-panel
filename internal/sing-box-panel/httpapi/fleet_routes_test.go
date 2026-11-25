// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestNodeRoutesConformToOpenAPI(t *testing.T) {
	router, err := legacy.NewRouter(loadOpenAPIContract(t))
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Email, value.Auth.PasswordHash = testutil.AdminEmail, testutil.PasswordHash
	value = settingsFileFixture(t, value)
	handler := newTestHandler(t, HandlerOptions{Settings: value, Build: buildinfo.Info{Version: "test"}, Services: newTestServices(t, database, value)})

	created := serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/nodes", `{"name":"edge"}`, http.StatusCreated, true, nil)
	if created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("enrollment response may be cached: %v", created.Header())
	}
	var enrollment fleet.Enrollment
	if err := json.Unmarshal(created.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	code, err := nodeapi.ParseEnrollmentCode(enrollment.Code)
	if err != nil || code.URL != "http://127.0.0.1:3000" || enrollment.Name != "edge" {
		t.Fatalf("enrollment = %+v, code %+v, %v", enrollment, code, err)
	}
	// The node exists once a host enrolls with its code; a canceled code
	// creates nothing, and canceling again succeeds.
	node := "/api/v1/nodes/" + enrollment.NodeID
	serveConformingRequest(t, router, handler, http.MethodGet, node, "", http.StatusNotFound, true, nil)
	serveConformingRequest(t, router, handler, http.MethodDelete, node+"/enrollment", "", http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodDelete, node+"/enrollment", "", http.StatusNoContent, true, nil)
	// Only the cancellation that discarded a code is recorded.
	logs := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/logs/panel?search_codes=node.enrollment.cancel.completed", "", "")
	var page store.PanelLogPage
	if logs.Code != http.StatusOK || json.Unmarshal(logs.Body.Bytes(), &page) != nil || page.Total != 1 {
		t.Fatalf("cancellation logs: %d %s", logs.Code, logs.Body.String())
	}
	enroll := func(token string) error {
		_, _, err := database.EnrollNode(t.Context(), nodeapi.TokenHash(token), store.NodeEnrollment{PublicKey: make([]byte, 32), EnrolledAt: time.Now().UTC()})
		return err
	}
	if err := enroll(code.Token); !errors.Is(err, store.ErrEnrollmentInvalid) {
		t.Fatalf("enrolling with a canceled code = %v, want ErrEnrollmentInvalid", err)
	}
	created = serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/nodes", `{"name":"edge"}`, http.StatusCreated, true, nil)
	if err := json.Unmarshal(created.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	if code, err = nodeapi.ParseEnrollmentCode(enrollment.Code); err != nil {
		t.Fatal(err)
	}
	if err := enroll(code.Token); err != nil {
		t.Fatal(err)
	}
	node = "/api/v1/nodes/" + enrollment.NodeID
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/nodes", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, node, `{"name":"edge-1"}`, http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/nodes", `{"name":"EDGE-1"}`, http.StatusConflict, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/nodes", `{"name":" edge"}`, http.StatusUnprocessableEntity, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, node+"/enrollment", "", http.StatusCreated, true, nil)
	serveConformingRequest(t, router, handler, http.MethodDelete, node+"/enrollment", "", http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, node, "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodDelete, node, "", http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, node, "", http.StatusNotFound, true, nil)
}

// failingDownloader fails every download of an official archive.
type failingDownloader struct{}

func (failingDownloader) Download(context.Context, string, io.Writer, int64) (int64, error) {
	return 0, errors.New("download unavailable")
}

// fleetRoutesFixture serves the management API over a database with nodes
// alpha, tagged eu, and beta, both offline without a recorded platform.
// Official archives cannot be downloaded.
func fleetRoutesFixture(t *testing.T) (routers.Router, *Handler, *store.Store, string, string) {
	t.Helper()
	router, err := legacy.NewRouter(loadOpenAPIContract(t))
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Email, value.Auth.PasswordHash = testutil.AdminEmail, testutil.PasswordHash
	value = settingsFileFixture(t, value)
	handler := newTestHandler(t, HandlerOptions{Settings: value, Build: buildinfo.Info{Version: "test"},
		Services: app.New(database, value, app.Options{Downloader: failingDownloader{}})})
	alpha, beta := testutil.CreateNode(t, database, "alpha"), testutil.CreateNode(t, database, "beta")
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/nodes/"+alpha+"/settings",
		`{"revision":1,"public_host":"","traffic":{"quota_gib":null,"period_months":1,"reset_day":1},"tags":["eu"]}`, http.StatusOK, true, nil)
	return router, handler, database, alpha, beta
}

// statusItems is a fleet status page or stream frame as a client reads it.
type statusItems struct {
	Reset      bool               `json:"reset"`
	Items      []fleet.NodeStatus `json:"items"`
	Removed    []string           `json:"removed"`
	NextCursor string             `json:"next_cursor"`
}

func (items statusItems) ids() []string {
	ids := make([]string, len(items.Items))
	for index, item := range items.Items {
		ids[index] = item.Node.ID
	}
	return ids
}

// serverEvent is one server-sent event.
type serverEvent struct {
	name, data string
}

// readServerEvent returns the next event, skipping comments.
func readServerEvent(t *testing.T, reader *bufio.Reader) serverEvent {
	t.Helper()
	var event serverEvent
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "" && event.name != "":
			return event
		case strings.HasPrefix(line, "event: "):
			event.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			event.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestFleetStreamSendsChangesAndStatusPages(t *testing.T) {
	router, handler, _, alpha, beta := fleetRoutesFixture(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Invalid filters fail before the stream opens; the stream does not page.
	for _, query := range []string{"state=pending", "tag=EU", "limit=1", "cursor=x", "sort=name"} {
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/fleet/stream?"+query, "", "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("stream ?%s status=%d body=%s", query, response.Code, response.Body.String())
		}
	}

	// Without a stream every read is new, and an unchanged fleet keeps its
	// encoding, so the same page answers 304.
	first := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/fleet/status?state=offline", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/fleet/status?state=offline", "", http.StatusNotModified, true,
		map[string]string{"If-None-Match": first.Header().Get("ETag")})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/fleet/stream?tag=eu", nil)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Authorize(t, handler, request)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream status=%d headers=%v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	frame := func() statusItems {
		t.Helper()
		event := readServerEvent(t, reader)
		var items statusItems
		if err := json.Unmarshal([]byte(event.data), &items); err != nil || event.name != "fleet" {
			t.Fatalf("event %q = %s, %v", event.name, event.data, err)
		}
		return items
	}
	// The first frame holds every selected node, then only changes follow.
	if first := frame(); !first.Reset || !slices.Equal(first.ids(), []string{alpha}) || len(first.Removed) != 0 {
		t.Fatalf("first frame = %+v; want only the tagged node", first)
	}
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/nodes/"+alpha, `{"name":"alpha-1"}`, http.StatusOK, true, nil)
	if renamed := frame(); renamed.Reset || len(renamed.Items) != 1 || renamed.Items[0].Node.Name != "alpha-1" || len(renamed.Removed) != 0 {
		t.Fatalf("frame after the rename = %+v; want only the renamed node", renamed)
	}
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/nodes/"+alpha+"/settings",
		`{"revision":2,"public_host":"","traffic":{"quota_gib":null,"period_months":1,"reset_day":1},"tags":[]}`, http.StatusOK, true, nil)
	if untagged := frame(); untagged.Reset || len(untagged.Items) != 0 || !slices.Equal(untagged.Removed, []string{alpha}) {
		t.Fatalf("frame after removing the tag = %+v; want the node removed", untagged)
	}

	// Pages continue after the last node of the previous one.
	var page statusItems
	response1 := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/fleet/status?limit=1", "", http.StatusOK, true, nil)
	if err := json.Unmarshal(response1.Body.Bytes(), &page); err != nil || !slices.Equal(page.ids(), []string{alpha}) || page.NextCursor == "" {
		t.Fatalf("first page = %s, %v", response1.Body.String(), err)
	}
	response2 := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/fleet/status?limit=1&cursor="+page.NextCursor, "", http.StatusOK, true, nil)
	page = statusItems{}
	if err := json.Unmarshal(response2.Body.Bytes(), &page); err != nil || !slices.Equal(page.ids(), []string{beta}) || page.NextCursor != "" {
		t.Fatalf("second page = %s, %v", response2.Body.String(), err)
	}
}

// commandEvents runs a fleet command and returns its result events by node
// and its summary.
func commandEvents(t *testing.T, router routers.Router, handler *Handler, body string) (map[string]fleetCommandResult, fleetCommandSummary) {
	t.Helper()
	// The contract checks the request; event streams carry their schemas in
	// event data, decoded below.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/fleet/commands", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	testutil.Authorize(t, handler, request)
	if err := validateOpenAPIRequestValue(router, request); err != nil {
		t.Fatal(err)
	}
	request.Body = io.NopCloser(strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("fleet command status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	results := map[string]fleetCommandResult{}
	var summary fleetCommandSummary
	reader := bufio.NewReader(response.Body)
	for {
		event := readServerEvent(t, reader)
		switch event.name {
		case "result":
			var result fleetCommandResult
			if err := json.Unmarshal([]byte(event.data), &result); err != nil {
				t.Fatal(err)
			}
			results[result.NodeID] = result
		case "done":
			if err := json.Unmarshal([]byte(event.data), &summary); err != nil {
				t.Fatal(err)
			}
			return results, summary
		default:
			t.Fatalf("unexpected event %+v", event)
		}
	}
}

func TestFleetCommandsStreamEveryNodesOutcome(t *testing.T) {
	router, handler, database, alpha, beta := fleetRoutesFixture(t)
	for _, body := range []string{
		`{"action":"stop"}`,
		`{"action":"stop","node_ids":["` + alpha + `"],"tag":"eu"}`,
		`{"action":"stop","tag":"eu","exact_version":"1.13.19"}`,
		`{"action":"install_core","tag":"eu"}`,
	} {
		serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/fleet/commands", body, http.StatusUnprocessableEntity, true, nil)
	}
	for _, body := range []string{`{"action":"stop","node_ids":[]}`, `{"action":"reboot","tag":"eu"}`, `{"action":"stop","tag":"EU"}`} {
		if response := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/fleet/commands", body, ""); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("fleet command %s status=%d body=%s", body, response.Code, response.Body.String())
		}
	}

	// Nodes that cannot run the command fail on their own; an ID of no node
	// is that node's failure, and a repeated ID runs once.
	results, summary := commandEvents(t, router, handler, `{"action":"stop","node_ids":["`+beta+`","node_missing","`+beta+`"]}`)
	want := map[string]fleetCommandResult{
		beta:           {NodeID: beta, Code: "node_offline", Title: "Node offline"},
		"node_missing": {NodeID: "node_missing", Code: "node_not_found", Title: "Node not found"},
	}
	if !maps.Equal(results, want) || summary != (fleetCommandSummary{Total: 2, Failed: 2}) {
		t.Fatalf("results = %+v, summary %+v; want %+v", results, summary, want)
	}

	// Installing a version picks each node's own platform's archive: the
	// catalog has one for linux/amd64 only, which cannot be downloaded.
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/fleet/commands",
		`{"action":"install_core","tag":"eu","exact_version":"1.13.19"}`, http.StatusConflict, true, nil)
	seedCoreHTTPCatalog(t, database)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/fleet/commands",
		`{"action":"install_core","tag":"eu","exact_version":"1.13.20"}`, http.StatusUnprocessableEntity, true, nil)
	enrolled := map[string]string{}
	for name, arch := range map[string]string{"arm": "arm64", "amd": "amd64"} {
		enrolled[arch], _ = testutil.EnrollNode(t, database, name, store.NodeEnrollment{OS: "linux", Arch: arch})
	}
	results, summary = commandEvents(t, router, handler,
		`{"action":"install_core","node_ids":["`+alpha+`","`+enrolled["arm64"]+`","`+enrolled["amd64"]+`"],"exact_version":"1.13.19"}`)
	want = map[string]fleetCommandResult{
		alpha:             {NodeID: alpha, Code: "node_platform_unknown", Title: "Node platform unknown"},
		enrolled["arm64"]: {NodeID: enrolled["arm64"], Code: "core_asset_unavailable", Title: "Core archive unavailable"},
		enrolled["amd64"]: {NodeID: enrolled["amd64"], Code: "core_download_failed", Title: "Core download failed"},
	}
	if !maps.Equal(results, want) || summary != (fleetCommandSummary{Total: 3, Failed: 3}) {
		t.Fatalf("install results = %+v, summary %+v; want %+v", results, summary, want)
	}
}
