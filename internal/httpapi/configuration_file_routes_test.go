// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestNewInboundDefaultsRequireAuthenticationAndAreNotCached(t *testing.T) {
	_, app, handler := newSubscriptionHTTPServices(t, "")
	view, _ := app.PanelSettings(context.Background())
	if _, err := app.SavePanelSettings(context.Background(), application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, IdentityKey: "fixture-identity"}); err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v1/config/inbound-defaults", strings.NewReader(`{"type":"anytls"}`)))
	if unauthorized.Code != 401 {
		t.Fatal("unprotected credentials")
	}
	response := authenticatedRequest(handler, http.MethodPost, "/api/v1/config/inbound-defaults", `{"type":"anytls"}`, "")
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), "fixture-identity") {
		t.Fatalf("defaults: %d", response.Code)
	}
	response = authenticatedRequest(handler, http.MethodPost, "/api/v1/config/inbound-defaults", `{"type":"anytls","unknown":true}`, "")
	if response.Code == 200 {
		t.Fatal("accepted unexpected fields")
	}
}

func TestConfigurationFileHTTPPreservesIncompleteJSONAndRequiresCAS(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	config := settings.Defaults()
	config.Auth.Token = strings.Repeat("a", 32)
	handler := NewHandler(HandlerOptions{Settings: config, Commands: application.FromStoreWithSettings(db, config)})
	request := func(method, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/config/file", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if response := request(http.MethodGet, "", ""); response.Code != 401 {
		t.Fatalf("unauthenticated=%d", response.Code)
	}
	body, _ := json.Marshal(application.ConfigurationFileWrite{Revision: 0, Content: "{\n \"future\":"})
	response := request(http.MethodPut, string(body), config.Auth.Token)
	if response.Code != 200 {
		t.Fatalf("save=%d %s", response.Code, response.Body.String())
	}
	var saved application.ConfigurationFile
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SyntaxValid || saved.Content != "{\n \"future\":" {
		t.Fatalf("saved=%+v", saved)
	}
	if response = request(http.MethodGet, "", config.Auth.Token); response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("get=%d", response.Code)
	}
	if response = request(http.MethodPut, string(body), config.Auth.Token); response.Code != 412 {
		t.Fatalf("stale=%d", response.Code)
	}
	if response = request(http.MethodPut, `{"revision":1,"content":"{}","unexpected":true}`, config.Auth.Token); response.Code == 200 {
		t.Fatal("unknown field accepted")
	}
}
