// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

func (handler *Handler) login(w http.ResponseWriter, request *http.Request) {
	client := loginClient(request.RemoteAddr)
	if allowed, retryAfter := handler.logins.allow(client); !allowed {
		seconds := max(1, int((retryAfter+time.Second-1)/time.Second))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeProblem(w, request, http.StatusTooManyRequests, "login_rate_limited", "Too many login attempts", "Wait before trying to authenticate again.")
		return
	}
	data, err := readBoundedBody(request, maxLoginBody)
	if err != nil {
		handler.logins.failed(client)
		writeProblem(w, request, http.StatusBadRequest, "invalid_body", "Invalid request", err.Error())
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := jsonstrict.Decode(data, maxLoginBody, &input); err != nil {
		handler.logins.failed(client)
		writeProblem(w, request, http.StatusBadRequest, "invalid_login", "Invalid login", "The login payload is invalid.")
		return
	}
	if !constantTimeTokenEqual(input.Token, handler.settings.Auth.Token) {
		handler.logins.failed(client)
		writeProblem(w, request, http.StatusUnauthorized, "invalid_credentials", "Authentication failed", "The supplied management token is invalid.")
		return
	}
	handler.logins.succeeded(client)
	raw, csrf, expiresAt, err := handler.sessions.create()
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "session_failed", "Session creation failed", "A secure session could not be created.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: raw, Path: cookiePath(handler.settings.Server.BasePath),
		Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()), HttpOnly: true,
		Secure: handler.settings.Auth.SecureCookie, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, sessionPayload{DisplayName: "Administrator", CSRFToken: csrf, ExpiresAt: expiresAt.UTC()})
}

type sessionPayload struct {
	DisplayName string    `json:"displayName"`
	CSRFToken   string    `json:"csrfToken,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
}

func (handler *Handler) currentSession(w http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
		writeJSON(w, http.StatusOK, sessionPayload{DisplayName: "Administrator"})
		return
	}
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		writeProblem(w, request, http.StatusUnauthorized, "authentication_required", "Authentication required", "A valid management session is required.")
		return
	}
	current, ok := handler.sessions.find(cookie.Value)
	if !ok {
		writeProblem(w, request, http.StatusUnauthorized, "session_expired", "Session expired", "The management session is missing or expired.")
		return
	}
	writeJSON(w, http.StatusOK, sessionPayload{DisplayName: "Administrator", CSRFToken: current.csrf, ExpiresAt: current.expiresAt.UTC()})
}

func (handler *Handler) logout(w http.ResponseWriter, request *http.Request) {
	cookie, _ := request.Cookie(sessionCookie)
	if cookie != nil {
		handler.sessions.delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Path: cookiePath(handler.settings.Server.BasePath), MaxAge: -1,
		HttpOnly: true, Secure: handler.settings.Auth.SecureCookie, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		authorization := request.Header.Get("Authorization")
		if strings.HasPrefix(authorization, "Bearer ") {
			bearer := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
			if constantTimeTokenEqual(bearer, handler.settings.Auth.Token) {
				next(w, request)
				return
			}
		}
		cookie, err := request.Cookie(sessionCookie)
		if err != nil {
			writeProblem(w, request, http.StatusUnauthorized, "authentication_required", "Authentication required", "A valid management session or bearer token is required.")
			return
		}
		session, ok := handler.sessions.find(cookie.Value)
		if !ok {
			writeProblem(w, request, http.StatusUnauthorized, "session_expired", "Session expired", "The management session is missing or expired.")
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions {
			if !constantTimeTokenEqual(request.Header.Get("X-CSRF-Token"), session.csrf) || !handler.sameOrigin(request) {
				writeProblem(w, request, http.StatusForbidden, "csrf_failed", "Request rejected", "The CSRF token or request origin is invalid.")
				return
			}
		}
		next(w, request)
	}
}

func (handler *Handler) sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return false
	}
	actual, err := settings.NormalizeOrigin(origin)
	if err != nil {
		return false
	}
	if handler.settings.Server.ExternalOrigin != "" {
		return actual == handler.settings.Server.ExternalOrigin
	}
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	expected, err := settings.NormalizeOrigin(scheme + "://" + request.Host)
	return err == nil && actual == expected
}

func cookiePath(base string) string {
	if base == "" {
		return "/"
	}
	return base + "/"
}
