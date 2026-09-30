// SPDX-License-Identifier: GPL-3.0-or-later
package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Public credentials for isolated test instances only.
const AdminEmail = "admin@example.com"
const AdminPassword = "test-administrator-password"
const PasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$oga1i5eT0bRdhmq1wUuE7Q$f4wS4NFRybHxufqO1R5BZLSGSyv4q1FTvYgpijrDjh0"
const ChangedPassword = "changed-administrator-password"
const ChangedPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$ZoLGjuSGdV0OFtsJ7pmIRA$ku1vDb7RNlBcjO0AggdzdDv0JU1WiRRlF5lBst96V24"

// Authorize uses the public login boundary, including Origin and CSRF. It never
// inserts sessions into storage or bypasses authentication middleware.
func Authorize(t testing.TB, handler http.Handler, request *http.Request) {
	t.Helper()
	base, _, found := strings.Cut(request.URL.Path, "/api/v1/")
	if !found {
		base = ""
	}
	origin := request.Header.Get("Origin")
	if origin == "" {
		scheme := "http"
		if request.TLS != nil {
			scheme = "https"
		}
		origin = scheme + "://" + request.Host
	}
	body, err := json.Marshal(map[string]any{"email": AdminEmail, "password": AdminPassword})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRequest(http.MethodPost, base+"/api/v1/auth/session", strings.NewReader(string(body)))
	login.Host = request.Host
	login.TLS = request.TLS
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("Origin", origin)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticate test request: status=%d body=%s", response.Code, response.Body.String())
	}
	var session struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range response.Result().Cookies() {
		request.AddCookie(cookie)
	}
	request.Header.Set("X-CSRF-Token", session.CSRF)
	request.Header.Set("Origin", origin)
}
