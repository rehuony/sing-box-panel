// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/filesystem"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

func TestFilesystemHTTPContract(t *testing.T) {
	value := settings.Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Token = "filesystem-test"
	value.Server.BasePath = "/panel"
	runtime := filepath.Join(value.DataDir, "runtime")
	if err := os.Mkdir(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	file := "中文 #key.pem"
	if err := os.WriteFile(filepath.Join(runtime, file), []byte("private file content"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(HandlerOptions{Settings: value, Commands: application.FromStoreWithSettings(nil, value)})
	for _, route := range []string{"entries", "resolve?mode=file&path=" + url.QueryEscape(file)} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panel/api/v1/filesystem/"+route, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: %d", route, recorder.Code)
		}
	}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"entries", 200}, {"entries?limit=1&show_hidden=true", 200},
		{"entries?limit=0", 400}, {"entries?path=a&path=b", 400}, {"entries?unknown=yes", 400},
		{"resolve?mode=file&path=" + url.QueryEscape(file), 200},
		{"resolve?mode=output-file&path=new.log", 200}, {"resolve?mode=file&path=absent", 404},
		{"resolve?mode=directory&path=" + url.QueryEscape(file), 422}, {"resolve?mode=bad&path=.", 400},
		{"resolve", 400},
	} {
		request := httptest.NewRequest(http.MethodGet, "/panel/api/v1/filesystem/"+tc.path, nil)
		request.Header.Set("Authorization", "Bearer "+value.Auth.Token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", tc.path, response.Code, response.Body.String())
		}
		if tc.path == "entries" {
			var page filesystem.Page
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Path != runtime || len(page.Items) != 1 || page.Items[0].Name != file {
				t.Fatalf("wrong runtime directory: %+v", page)
			}
		}
	}
	value.Auth.Token = "openapi-response-test"
	contractHandler := NewHandler(HandlerOptions{Settings: settings.Settings{DataDir: value.DataDir, Auth: settings.Auth{Token: "openapi-response-test"}}, Commands: application.FromStoreWithSettings(nil, value)})
	router, err := legacy.NewRouter(loadOpenAPIContract(t))
	if err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, contractHandler, http.MethodGet, "/api/v1/filesystem/entries", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, contractHandler, http.MethodGet, "/api/v1/filesystem/resolve?mode=file&path="+url.QueryEscape(file), "", http.StatusOK, true, nil)
}
