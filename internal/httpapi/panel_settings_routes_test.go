package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func TestPanelSettingsRemovedFieldsDoNotRoundTrip(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	configuration := settingsFileFixture(t, settings.Defaults())
	app := application.FromStoreWithSettings(database, configuration)
	handler := newTestHandler(t, HandlerOptions{Settings: configuration, Commands: app})
	request := func(method string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/panel/settings", bytes.NewReader(body))
		testutil.Authorize(t, handler, req)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	response := request(http.MethodGet, nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var view application.PanelSettingsView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"subscription_author", "subscription_provider", "log_retention_days", "secure_cookie"} {
		if strings.Contains(response.Body.String(), `"`+field+`"`) {
			t.Fatalf("GET returned removed field %s", field)
		}
		body, _ := json.Marshal(application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Service: &view.Service})
		value := `"obsolete"`
		if field == "log_retention_days" {
			value = "7"
		} else if field == "secure_cookie" {
			value = "false"
		}
		body = bytes.Replace(body, []byte(`"service":{`), []byte(`"service":{"`+field+`":`+value+`,`), 1)
		assertCoreHTTPProblem(t, request(http.MethodPut, body), http.StatusUnprocessableEntity, "invalid_json")
	}
	unchanged, err := app.PanelSettings(t.Context())
	if err != nil || unchanged.Revision != view.Revision {
		t.Fatal("invalid writes changed settings", err)
	}
	view.Preferences.Language = "en"
	body, _ := json.Marshal(application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Service: &view.Service})
	response = request(http.MethodPut, body)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	backup, err := app.ExportPanelBackup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"author"`, `"provider"`, `"retention_days"`, `"secure_cookie"`} {
		if bytes.Contains(backup.PanelSettings, []byte(field)) {
			t.Fatalf("settings save regenerated %s", field)
		}
	}
}

func loginWithPassword(t *testing.T, handler http.Handler, password string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(application.LoginInput{Email: testutil.AdminEmail, Password: password})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.com")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestPanelSettingsAuthenticationPersistenceAndRotation(t *testing.T) {
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	configuration := settingsFileFixture(t, settings.Defaults())
	app := application.FromStoreWithSettings(db, configuration)
	handler := newTestHandler(t, HandlerOptions{Settings: configuration, Commands: app})
	login := loginWithPassword(t, handler, testutil.AdminPassword)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	var session sessionPayload
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	input := application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Credentials: &application.CredentialsWrite{Email: new(testutil.AdminEmail), NewPassword: testutil.ChangedPassword}, GitHubToken: "github-test-secret"}
	save := func(csrf bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		request := httptest.NewRequest(http.MethodPut, "/api/v1/panel/settings", bytes.NewReader(body))
		request.AddCookie(cookie)
		request.Header.Set("Content-Type", "application/json")
		if csrf {
			request.Header.Set("X-CSRF-Token", session.CSRFToken)
			request.Header.Set("Origin", "http://example.com")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if got := save(false); got.Code != 403 {
		t.Fatalf("missing CSRF: %d", got.Code)
	}
	input.Credentials.NewPassword = "short"
	assertCoreHTTPProblem(t, save(true), 422, "panel_settings_invalid")
	unchanged, err := app.PanelSettings(t.Context())
	if err != nil || unchanged.Revision != view.Revision || unchanged.GitHubTokenConfigured {
		t.Fatal("failed save changed configuration", err)
	}
	if _, err := app.CurrentSession(t.Context(), cookie.Value); err != nil {
		t.Fatal("failed save invalidated session", err)
	}
	input.Credentials.NewPassword = testutil.ChangedPassword
	response := save(true)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var result application.PanelSettingsSaveResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.ReauthenticationRequired {
		t.Fatalf("save: %+v %v", result, err)
	}
	for _, secret := range []string{testutil.ChangedPassword, testutil.AdminPassword, input.GitHubToken, testutil.PasswordHash} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("secret leaked")
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/panel/settings", nil)
	request.AddCookie(cookie)
	got := httptest.NewRecorder()
	handler.ServeHTTP(got, request)
	if got.Code != 401 {
		t.Fatalf("old session: %d", got.Code)
	}
	if got := loginWithPassword(t, handler, testutil.AdminPassword); got.Code != 401 {
		t.Fatalf("old password: %d", got.Code)
	}
	if got := loginWithPassword(t, handler, testutil.ChangedPassword); got.Code != 200 {
		t.Fatalf("new password: %d %s", got.Code, got.Body.String())
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
	handler := newTestHandler(t, HandlerOptions{Settings: configuration, Commands: app})
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
	testutil.Authorize(t, handler, request)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertCoreHTTPProblem(t, response, http.StatusUnprocessableEntity, "invalid_json")
}

func TestPanelSettingsPasswordRotationRemainsUsable(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		valid          bool
	}{
		{"minimum", "123456789012", true}, {"unicode minimum", strings.Repeat("密", 12), true},
		{"short", strings.Repeat("密", 11), false}, {"maximum", strings.Repeat("密", 128), true},
		{"too long", strings.Repeat("x", 129), false}, {"whitespace preserved", "  pass word with spaces  ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			cfg := settingsFileFixture(t, settings.Defaults())
			app := application.FromStoreWithSettings(db, cfg)
			handler := newTestHandler(t, HandlerOptions{Settings: cfg, Commands: app})
			view, err := app.PanelSettings(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Credentials: &application.CredentialsWrite{NewPassword: tc.password}})
			request := httptest.NewRequest(http.MethodPut, "/api/v1/panel/settings", bytes.NewReader(body))
			testutil.Authorize(t, handler, request)
			saved := httptest.NewRecorder()
			handler.ServeHTTP(saved, request)
			if tc.valid {
				if saved.Code != 200 {
					t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
				}
				if got := loginWithPassword(t, handler, tc.password); got.Code != 200 {
					t.Fatalf("login: %d %s", got.Code, got.Body.String())
				}
			} else {
				if saved.Code != 422 {
					t.Fatalf("invalid password: %d", saved.Code)
				}
				if got := loginWithPassword(t, handler, testutil.AdminPassword); got.Code != 200 {
					t.Fatalf("original login: %d", got.Code)
				}
				after, err := app.PanelSettings(t.Context())
				if err != nil || after.Revision != view.Revision {
					t.Fatal("rejected write changed settings", err)
				}
			}
		})
	}
}

func TestManualSettingsPasswordEditChangesAuthentication(t *testing.T) {
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := settingsFileFixture(t, settings.Defaults())
	app := application.FromStoreWithSettings(db, value)
	handler := newTestHandler(t, HandlerOptions{Settings: value, Commands: app})
	login := loginWithPassword(t, handler, testutil.AdminPassword)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	value.Auth.PasswordHash = testutil.ChangedPasswordHash
	raw, _ := json.Marshal(value)
	if err := settings.Replace(value.Path(), raw); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/panel/settings", nil)
	request.AddCookie(login.Result().Cookies()[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatalf("old session: %d", response.Code)
	}
	if got := loginWithPassword(t, handler, testutil.AdminPassword); got.Code != 401 {
		t.Fatalf("old password: %d", got.Code)
	}
	if got := loginWithPassword(t, handler, testutil.ChangedPassword); got.Code != 200 {
		t.Fatalf("new password: %d", got.Code)
	}
}

func TestPanelSettingsRejectsRemovedIdentityFields(t *testing.T) {
	_, app, handler := newSubscriptionHTTPServices(t, "")
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/panel/settings", "", "")
	for _, field := range []string{"identity_name", "identity_key", "identity_key_configured"} {
		if strings.Contains(response.Body.String(), field) {
			t.Fatalf("settings response retained %s", field)
		}
	}
	for _, field := range []string{"identity_name", "identity_key", "clear_identity_key"} {
		t.Run(field, func(t *testing.T) {
			raw, err := json.Marshal(application.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences})
			if err != nil {
				t.Fatal(err)
			}
			var input map[string]any
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "identity_name":
				input["preferences"].(map[string]any)[field] = "removed"
			case "clear_identity_key":
				input[field] = true
			default:
				input[field] = "removed"
			}
			raw, _ = json.Marshal(input)
			response := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/panel/settings", string(raw), "")
			assertCoreHTTPProblem(t, response, http.StatusUnprocessableEntity, "invalid_json")
			after, err := app.PanelSettings(t.Context())
			if err != nil || after.Revision != view.Revision {
				t.Fatal("rejected input changed settings", err)
			}
		})
	}
}
