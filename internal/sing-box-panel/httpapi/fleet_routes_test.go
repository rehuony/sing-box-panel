// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
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
	if code, err := nodeapi.ParseEnrollmentCode(enrollment.Code); err != nil || code.Pin == "" || code.Endpoint != "127.0.0.1:3000" || code.Path != "" {
		t.Fatalf("enrollment code = %+v, %v", code, err)
	}
	node := "/api/v1/nodes/" + enrollment.Node.ID
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/nodes", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, node, `{"name":"edge-1"}`, http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/nodes", `{"name":"EDGE-1"}`, http.StatusConflict, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/nodes", `{"name":" edge"}`, http.StatusUnprocessableEntity, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, node+"/enrollment", "", http.StatusCreated, true, nil)
	serveConformingRequest(t, router, handler, http.MethodDelete, node, "", http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, node, "", http.StatusNotFound, true, nil)
}
