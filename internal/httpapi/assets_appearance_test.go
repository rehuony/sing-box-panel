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
		"index.html": &fstest.MapFile{Data: []byte(`<meta name="sing-box-panel-appearance" content="__SBP_APPEARANCE__" /><style nonce="__SBP_STYLE_NONCE__">/*__SBP_APPEARANCE_CSS__*/</style>`)},
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
			_, nonce, _ := strings.Cut(policy, "'nonce-")
			nonce, _, _ = strings.Cut(nonce, "'")
			if nonce == "" || !strings.Contains(body, `<style nonce="`+nonce+`">`+initialAppearanceCSS(want)+`</style>`) {
				t.Fatal("initial palette is missing or not authorized by the style policy")
			}
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

func TestInitialAppearancePalette(t *testing.T) {
	// These values also appear in the frontend appearance tests to guard the
	// first-paint/React handoff against changes to either side's blend rules.
	light := "color-scheme:light;--color-paper:#fefcfa;--color-paper-2:#fdf8f6;--color-paper-3:#faf2ec;--color-accent-soft:#faf0ea;--color-canvas:var(--color-paper-2);--color-canvas-deep:var(--color-paper-3);"
	dark := "color-scheme:dark;--color-paper:#1b181e;--color-paper-2:#1f1a1e;--color-paper-3:#261c1d;--color-accent-soft:#34221c;--color-canvas:var(--color-paper);--color-canvas-deep:oklch(13% 0.014 274);"
	for _, theme := range []string{"light", "dark", "system"} {
		css := initialAppearanceCSS(settings.Appearance{Theme: theme, Color: "#C65B13", Radius: 12})
		if (theme != "dark" && !strings.Contains(css, light)) || (theme != "light" && !strings.Contains(css, dark)) {
			t.Fatalf("%s palette = %s", theme, css)
		}
		if strings.Contains(css, "@media(prefers-color-scheme:dark)") != (theme == "system") {
			t.Fatalf("unexpected system theme override: %s", css)
		}
	}
	unsafe := initialAppearanceCSS(settings.Appearance{Theme: "system", Color: "</style><script>bad</script>"})
	if strings.Contains(unsafe, "<") || !strings.Contains(unsafe, "color-scheme:dark") {
		t.Fatalf("invalid appearance did not fall back safely: %s", unsafe)
	}
}
