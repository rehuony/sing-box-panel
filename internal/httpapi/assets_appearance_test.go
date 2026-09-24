// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestAnonymousHTMLSharesCurrentPanelAppearance(t *testing.T) {
	value := settings.Defaults()
	value.DataDir = t.TempDir()
	value.Server.BasePath = "/panel"
	value.Auth.Token = "private-management-token"
	value.GitHub.Token = "private-github-token"
	value.Panel.Appearance = settings.Appearance{Theme: "dark", Color: "#C65B13", Radius: 8}
	value = settingsFileFixture(t, value)
	database, err := store.Open(t.Context(), filepath.Join(value.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := application.FromStoreWithSettings(database, value)
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<meta name="sing-box-panel-appearance" content="__SBP_APPEARANCE__" />`)},
	}
	handler := NewHandler(HandlerOptions{Settings: value, Commands: app, Assets: assets})
	assertAppearance := func(want settings.Appearance) {
		t.Helper()
		for _, path := range []string{"/panel/login", "/panel/", "/panel/configuration"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("anonymous HTML: status=%d headers=%v", response.Code, response.Header())
			}
			body := response.Body.String()
			_, content, found := strings.Cut(body, `content="`)
			content, _, closed := strings.Cut(content, `"`)
			var got map[string]any
			if !found || !closed || json.Unmarshal([]byte(html.UnescapeString(content)), &got) != nil {
				t.Fatalf("invalid appearance metadata: %s", body)
			}
			if len(got) != 3 || got["theme"] != want.Theme || got["color"] != want.Color || got["radius"] != float64(want.Radius) {
				t.Fatalf("appearance = %v; want %+v", got, want)
			}
			for _, private := range []string{value.Auth.Token, value.GitHub.Token, value.DataDir, "listen_host", "service"} {
				if strings.Contains(body, private) {
					t.Fatal("anonymous HTML contains private panel settings")
				}
			}
			policy := response.Header().Get("Content-Security-Policy")
			if !strings.Contains(policy, "script-src 'self'") || strings.Contains(policy, "'unsafe-inline'") {
				t.Fatalf("appearance weakened the script policy: %s", policy)
			}
		}
	}
	assertAppearance(value.Panel.Appearance)
	view, err := app.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	view.Preferences.Appearance = settings.Appearance{Theme: "system", Color: "#15803D", Radius: 0}
	if _, err := app.SavePanelSettings(t.Context(), application.PanelSettingsWrite{
		Revision: view.Revision, Preferences: view.Preferences,
	}); err != nil {
		t.Fatal(err)
	}
	assertAppearance(view.Preferences.Appearance)

	// A temporarily unreadable settings file must not prevent reaching login.
	if err := os.WriteFile(value.Path(), []byte("invalid settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertAppearance(value.Panel.Appearance)
}
