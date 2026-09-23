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
)

func TestPanelBackupAuthenticationCSRFAndRevisionConflict(t *testing.T) {
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := settingsFileFixture(t, settings.Defaults())
	app := application.FromStoreWithSettings(db, cfg)
	handler := NewHandler(HandlerOptions{Settings: cfg, Commands: app})
	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/v1/panel/backup", nil))
	if anonymous.Code != 401 {
		t.Fatalf("anonymous export: %d", anonymous.Code)
	}
	login := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"token": cfg.Auth.Token})
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", bytes.NewReader(body)))
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	var session sessionPayload
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0]
	request := httptest.NewRequest(http.MethodGet, "/api/v1/panel/backup", nil)
	request.AddCookie(cookie)
	exported := httptest.NewRecorder()
	handler.ServeHTTP(exported, request)
	if exported.Code != 200 || exported.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("export: %d %s", exported.Code, exported.Body.String())
	}
	var backup application.PanelBackup
	if err := json.Unmarshal(exported.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}
	view, _ := app.PanelSettings(t.Context())
	file, _ := app.ConfigurationFile(t.Context())
	backup.SingBoxConfiguration = "  exact raw text\n"
	input := application.PanelRestoreRequest{Backup: backup, SettingsRevision: view.Revision, ConfigurationRevision: file.Revision}
	restore := func(csrf bool) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/panel/restore", bytes.NewReader(encoded))
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
	if got := restore(false); got.Code != 403 {
		t.Fatalf("CSRF: %d", got.Code)
	}
	if got := restore(true); got.Code != 200 || strings.Contains(got.Body.String(), cfg.Auth.Token) {
		t.Fatalf("restore: %d", got.Code)
	}
	if got := restore(true); got.Code != 412 {
		t.Fatalf("stale revisions: %d", got.Code)
	}
	stored, _ := app.ConfigurationFile(t.Context())
	if stored.Content != backup.SingBoxConfiguration {
		t.Fatal("text was altered")
	}
	input.Backup.Version = 2
	if got := restore(true); got.Code != 422 {
		t.Fatalf("unsupported: %d", got.Code)
	}
}
