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

func TestPanelSettingsAuthenticationPersistenceAndRotation(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	configuration := settings.Defaults()
	configuration.Auth.Token = strings.Repeat("original-", 4)
	configuration = settingsFileFixture(t, configuration)
	app := application.FromStoreWithSettings(db, configuration)
	handler := NewHandler(HandlerOptions{Settings: configuration, Commands: app})
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/panel/settings", nil))
	if unauthenticated.Code != 401 {
		t.Fatalf("unauthenticated settings: %d", unauthenticated.Code)
	}
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"`+configuration.Auth.Token+`"}`)))
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	var session sessionPayload
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	view, err := app.PanelSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, ManagementToken: strings.Repeat("replacement-", 3), GitHubToken: "github-test-secret"}
	body, _ := json.Marshal(input)
	save := func(csrf bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/panel/settings", strings.NewReader(string(body)))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", "application/json")
		if csrf {
			req.Header.Set("X-CSRF-Token", session.CSRFToken)
			req.Header.Set("Origin", "http://example.com")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if got := save(false); got.Code != 403 {
		t.Fatalf("missing CSRF: %d", got.Code)
	}
	response := save(true)
	if response.Code != 200 {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), input.ManagementToken) || strings.Contains(response.Body.String(), input.GitHubToken) {
		t.Fatal("secret leaked")
	}
	for _, auth := range []string{"cookie", "old-token", "new-token"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/panel/settings", nil)
		switch auth {
		case "cookie":
			req.AddCookie(cookie)
		case "old-token":
			req.Header.Set("Authorization", "Bearer "+configuration.Auth.Token)
		case "new-token":
			req.Header.Set("Authorization", "Bearer "+input.ManagementToken)
		}
		got := httptest.NewRecorder()
		handler.ServeHTTP(got, req)
		want := 401
		if auth == "new-token" {
			want = 200
		}
		if got.Code != want {
			t.Fatalf("%s: %d, want %d", auth, got.Code, want)
		}
	}
}

func TestPanelSettingsRejectsRemovedCatalogTTLField(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	configuration := settingsFileFixture(t, settings.Defaults())
	app := application.FromStoreWithSettings(database, configuration)
	handler := NewHandler(HandlerOptions{Settings: configuration, Commands: app})
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(application.PanelSettingsWrite{
		Revision:    view.Revision,
		Preferences: view.Preferences,
		Service:     &view.Service,
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(string(body), "catalog_refresh_interval_hours", "catalog_ttl_hours", 1)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/panel/settings", strings.NewReader(legacy))
	request.Header.Set("Authorization", "Bearer "+configuration.Auth.Token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertCoreHTTPProblem(t, response, http.StatusUnprocessableEntity, "invalid_json")
}

func TestPanelSettingsTokenRotationRemainsUsable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		token string
		valid bool
	}{
		{"maximum length", strings.Repeat("x", 8192), true},
		{"maximum escaped length", strings.Repeat("<", 8192), true},
		{"leading space", " " + strings.Repeat("x", 32), false},
		{"trailing space", strings.Repeat("x", 32) + " ", false},
		{"trailing tab", strings.Repeat("x", 32) + "\t", false},
		{"Unicode space", strings.Repeat("x", 32) + "\u00a0", false},
		{"browser trimmed BOM", strings.Repeat("x", 32) + "\ufeff", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			cfg := settings.Defaults()
			cfg.Auth.Token = strings.Repeat("original-", 4)
			cfg = settingsFileFixture(t, cfg)
			app := application.FromStoreWithSettings(db, cfg)
			handler := NewHandler(HandlerOptions{Settings: cfg, Commands: app})
			login := func(token string) *httptest.ResponseRecorder {
				t.Helper()
				body, err := json.Marshal(map[string]string{"token": token})
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(string(body))))
				return response
			}
			original := login(cfg.Auth.Token)
			if original.Code != http.StatusOK {
				t.Fatalf("original login: %d", original.Code)
			}
			view, err := app.PanelSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, ManagementToken: tc.token})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPut, "/api/v1/panel/settings", strings.NewReader(string(body)))
			request.Header.Set("Authorization", "Bearer "+cfg.Auth.Token)
			saved := httptest.NewRecorder()
			handler.ServeHTTP(saved, request)
			if tc.valid {
				if saved.Code != http.StatusOK {
					t.Fatalf("save accepted token: %d", saved.Code)
				}
				if response := login(tc.token); response.Code != http.StatusOK {
					t.Fatalf("login with accepted token: %d", response.Code)
				}
				return
			}
			if saved.Code != http.StatusUnprocessableEntity {
				t.Fatalf("invalid token save: %d", saved.Code)
			}
			current, err := app.PanelSettings(ctx)
			if err != nil || current.Revision != view.Revision {
				t.Fatalf("rejected save changed revision: %+v, %v", current, err)
			}
			request = httptest.NewRequest(http.MethodGet, "/api/v1/panel/settings", nil)
			request.AddCookie(original.Result().Cookies()[0])
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || login(cfg.Auth.Token).Code != http.StatusOK {
				t.Fatal("rejected token rotation invalidated the original credentials")
			}
		})
	}
}

func TestManualSettingsTokenEditChangesAuthentication(t *testing.T) {
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := settingsFileFixture(t, settings.Defaults())
	app := application.FromStoreWithSettings(db, value)
	handler := NewHandler(HandlerOptions{Settings: value, Commands: app})
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"`+value.Auth.Token+`"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	oldToken := value.Auth.Token
	value.Auth.Token = strings.Repeat("replacement-", 3)
	raw, _ := json.Marshal(value)
	if err := settings.Replace(value.Path(), raw); err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{"cookie", oldToken, value.Auth.Token} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/panel/settings", nil)
		if credential == "cookie" {
			request.AddCookie(cookie)
		} else {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		want := http.StatusUnauthorized
		if credential == value.Auth.Token {
			want = http.StatusOK
		}
		if response.Code != want {
			t.Fatalf("authentication status = %d, want %d", response.Code, want)
		}
		if strings.Contains(response.Body.String(), value.Auth.Token) {
			t.Fatal("settings API exposed secret")
		}
	}
}
