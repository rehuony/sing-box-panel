// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

func TestFrontendAssetCachePolicy(t *testing.T) {
	config := settings.Defaults()
	config.DataDir = t.TempDir()
	config.Server.BasePath = "/panel"
	handler := NewHandler(HandlerOptions{Settings: config, Assets: fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte(`<base href="/" data-sbp-runtime />`)},
		"assets/app-Abc123_-.js":  &fstest.MapFile{Data: []byte(`export default {}`)},
		"assets/app-Abc123_-.css": &fstest.MapFile{Data: []byte(`body{margin:0}`)},
		"assets/app.js":           &fstest.MapFile{Data: []byte(`export default {}`)},
	}})
	for _, tc := range []struct{ path, cache string }{
		{"/panel/assets/app-Abc123_-.js", "public, max-age=31536000, immutable"},
		{"/panel/assets/app-Abc123_-.css", "public, max-age=31536000, immutable"},
		{"/panel/assets/app.js", ""},
		{"/panel/configuration", "no-store"},
		{"/panel/assets/missing-Abc123_-.js", "no-store"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("status=%d cache=%q, want 200 %q", response.Code, response.Header().Get("Cache-Control"), tc.cache)
			}
		})
	}
}
