// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestCookiePersistenceRotationAndRestart(t *testing.T) {
	for _, test := range []struct {
		name           string
		origin         string
		externalOrigin string
		transport      string
		basePath       string
		secure         bool
	}{
		{name: "http root", origin: "http://example.com", transport: "http://example.com"},
		{name: "direct TLS", origin: "https://example.com", transport: "https://example.com", basePath: "/panel", secure: true},
		{name: "HTTPS proxy over HTTP", origin: "https://example.com", externalOrigin: "https://example.com", transport: "http://localhost:3000", basePath: "/panel", secure: true},
		{name: "configured HTTP overrides TLS", origin: "http://example.com", externalOrigin: "http://example.com", transport: "https://localhost:3000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := settingsFileFixture(t, settings.Defaults())
			cfg.Server.BasePath = test.basePath
			cfg.Server.ExternalOrigin = test.externalOrigin
			newRequest := func(method, path string, body []byte) *http.Request {
				request := httptest.NewRequest(method, test.transport+test.basePath+path, bytes.NewReader(body))
				request.Header.Set("Origin", test.origin)
				// Opposing forwarded schemes must neither enable nor disable Secure.
				forwarded := "https"
				if test.secure {
					forwarded = "http"
				}
				request.Header.Set("Forwarded", "proto="+forwarded+";host=attacker.example")
				request.Header.Set("X-Forwarded-Proto", forwarded)
				return request
			}
			assertCleared := func(response *httptest.ResponseRecorder) {
				t.Helper()
				cookies := response.Result().Cookies()
				if len(cookies) != 1 || cookies[0].MaxAge != -1 || cookies[0].Secure != test.secure || cookies[0].Path != test.basePath+"/" {
					t.Fatal("cookie clearing did not use the active origin", cookies)
				}
			}
			path := filepath.Join(t.TempDir(), "panel.db")
			db, err := store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewHandler(HandlerOptions{Settings: cfg, Commands: application.FromStoreWithSettings(db, cfg)})
			loginEmail := testutil.AdminEmail
			login := func(previous *http.Cookie) *httptest.ResponseRecorder {
				body, _ := json.Marshal(application.LoginInput{Email: loginEmail, Password: testutil.AdminPassword})
				request := newRequest("POST", "/api/v1/auth/session", body)
				request.Header.Set("Content-Type", "application/json")
				if previous != nil {
					request.AddCookie(previous)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != 200 {
					t.Fatal(response.Body.String())
				}
				return response
			}
			earliestExpiry := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second)
			first := login(nil)
			latestExpiry := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second)
			cookie := first.Result().Cookies()[0]
			if !cookie.HttpOnly || cookie.Secure != test.secure || cookie.Path != test.basePath+"/" || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatal("cookie security properties", cookie)
			}
			if cookie.MaxAge != 604800 || cookie.Expires.Before(earliestExpiry) || cookie.Expires.After(latestExpiry) {
				t.Fatal("cookie must persist for exactly seven days", cookie)
			}
			var firstPayload application.AuthSession
			if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
				t.Fatal(err)
			}
			if !cookie.Expires.Equal(firstPayload.ExpiresAt) {
				t.Fatal("cookie and server expiration differ")
			}
			second := login(cookie)
			replacement := second.Result().Cookies()[0]
			var payload application.AuthSession
			if err := json.Unmarshal(second.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = store.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			handler = NewHandler(HandlerOptions{Settings: cfg, Commands: application.FromStoreWithSettings(db, cfg)})
			send := func(method string, cookie *http.Cookie) *httptest.ResponseRecorder {
				request := newRequest(method, "/api/v1/auth/session", nil)
				request.AddCookie(cookie)
				request.Header.Set("Origin", test.origin)
				request.Header.Set("X-CSRF-Token", payload.CSRFToken)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				return response
			}
			expired := send("GET", cookie)
			if expired.Code != 401 {
				t.Fatal("rotated cookie restored", expired.Code)
			}
			assertCleared(expired)
			restored := send("GET", replacement)
			if restored.Code != 200 || !bytes.Contains(restored.Body.Bytes(), []byte(payload.CSRFToken)) {
				t.Fatal("restart failed to restore csrf", restored.Code)
			}
			var restoredPayload application.AuthSession
			if err := json.Unmarshal(restored.Body.Bytes(), &restoredPayload); err != nil {
				t.Fatal(err)
			}
			if len(restored.Result().Cookies()) != 0 || !restoredPayload.ExpiresAt.Equal(payload.ExpiresAt) {
				t.Fatal("restoring a session must not renew it")
			}
			logout := send("DELETE", replacement)
			if logout.Code != 204 {
				t.Fatal("write after restart failed", logout.Code)
			}
			assertCleared(logout)
			if got := send("GET", replacement); got.Code != 401 {
				t.Fatal("logout did not revoke", got.Code)
			}
			current := login(nil)
			if err := json.Unmarshal(current.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			app := application.FromStoreWithSettings(db, cfg)
			backup, err := app.ExportPanelBackup(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			view, err := app.PanelSettings(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			// Saving a different origin does not activate it before restart.
			view.Preferences.ExternalOrigin = "http://changed.example.com"
			if !test.secure {
				view.Preferences.ExternalOrigin = "https://changed.example.com"
			}
			loginEmail = "用户@例子.测试"
			body, _ := json.Marshal(application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Credentials: &application.CredentialsWrite{Email: &loginEmail}})
			request := newRequest("PUT", "/api/v1/panel/settings", body)
			request.AddCookie(current.Result().Cookies()[0])
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", payload.CSRFToken)
			changed := httptest.NewRecorder()
			handler.ServeHTTP(changed, request)
			if changed.Code != 200 {
				t.Fatal("identity change failed", changed.Body.String())
			}
			assertCleared(changed)
			// The saved Unicode email must remain usable for authentication.
			current = login(nil)
			if err := json.Unmarshal(current.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			view, err = app.PanelSettings(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			configuration, err := app.ConfigurationFile(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			body, _ = json.Marshal(application.PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision, ConfigurationRevision: configuration.Revision})
			request = newRequest("POST", "/api/v1/panel/restore", body)
			request.AddCookie(current.Result().Cookies()[0])
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", payload.CSRFToken)
			restore := httptest.NewRecorder()
			handler.ServeHTTP(restore, request)
			if restore.Code != 200 {
				t.Fatal("identity restore failed", restore.Body.String())
			}
			assertCleared(restore)
			if got := send("GET", current.Result().Cookies()[0]); got.Code != 401 {
				t.Fatal("restore did not revoke session", got.Code)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if got := send("GET", replacement); got.Code != 503 || len(got.Result().Cookies()) != 0 {
				t.Fatal("database failure must not clear cookie", got.Code)
			}
		})
	}
}
