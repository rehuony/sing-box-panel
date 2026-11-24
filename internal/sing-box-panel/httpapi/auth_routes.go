// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

type sessionContextKey struct{}

func (handler *Handler) login(w http.ResponseWriter, request *http.Request) {
	login, rejection := handler.logins.Admit(w, request, handler.originAllowed, maxLoginBody)
	defer login.Finish()
	if rejection != nil {
		writeRejection(w, request, rejection)
		return
	}
	request = login.Request
	var input auth.LoginInput
	if err := jsonstrict.Decode(login.Body, maxLoginBody, &input); err != nil {
		writeProblem(w, request, http.StatusBadRequest, "invalid_login", "Invalid login", "The login payload is invalid.")
		return
	}
	if handler.services.Sessions == nil {
		handler.authenticationUnavailable(w, request)
		return
	}
	previous, _ := request.Cookie(sessionCookie)
	previousToken := ""
	if previous != nil {
		previousToken = previous.Value
	}
	value, err := handler.services.Sessions.Login(request.Context(), input, previousToken)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeProblem(w, request, http.StatusUnauthorized, "invalid_credentials", "Authentication failed", "The email or password is incorrect.")
		} else if errors.Is(err, webauth.ErrBusy) {
			w.Header().Set("Retry-After", "1")
			writeProblem(w, request, http.StatusTooManyRequests, "login_rate_limited", "Authentication busy", "Try again shortly.")
		} else {
			handler.authenticationUnavailable(w, request)
		}
		return
	}
	cookie := &http.Cookie{
		Name: sessionCookie, Value: value.Token, Path: cookiePath(handler.settings.Server.BasePath),
		HttpOnly: true, Secure: handler.secureCookie(request), SameSite: http.SameSiteStrictMode,
		Expires: value.ExpiresAt, MaxAge: int(auth.SessionLifetime.Seconds()),
	}
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusOK, value)
}

func (handler *Handler) currentSession(w http.ResponseWriter, request *http.Request) {
	value, ok := request.Context().Value(sessionContextKey{}).(auth.AuthSession)
	if !ok {
		handler.authenticationUnavailable(w, request)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

// originAllowed reports whether a login or write may proceed from the
// request's origin. Only a configured external origin is enforced; without
// one, the JSON-only login, the CSRF token and the SameSite cookie still stop
// cross-site requests, so a reverse proxy needs no origin setting.
func (handler *Handler) originAllowed(request *http.Request) bool {
	origin := handler.settings.Server.ExternalOrigin
	return origin == "" || webauth.FromOrigin(request, origin)
}

func (handler *Handler) secureCookie(request *http.Request) bool {
	// Use the same trusted origin as CSRF validation, never forwarded headers.
	if origin := handler.settings.Server.ExternalOrigin; origin != "" {
		return strings.HasPrefix(origin, "https://")
	}
	return request.TLS != nil
}

func (handler *Handler) clearSessionCookie(w http.ResponseWriter, request *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: cookiePath(handler.settings.Server.BasePath), MaxAge: -1, HttpOnly: true, Secure: handler.secureCookie(request), SameSite: http.SameSiteStrictMode})
}

func (handler *Handler) logout(w http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		handler.clearSessionCookie(w, request)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := handler.services.Sessions.Logout(request.Context(), cookie.Value); err != nil {
		handler.authenticationUnavailable(w, request)
		return
	}
	handler.clearSessionCookie(w, request)
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) authenticationUnavailable(w http.ResponseWriter, request *http.Request) {
	writeRejection(w, request, webauth.Unavailable())
}

func writeRejection(w http.ResponseWriter, request *http.Request, rejection *webauth.Rejection) {
	rejection.SetRetryAfter(w.Header())
	writeProblem(w, request, rejection.Status, rejection.Code, rejection.Title, rejection.Detail)
}

func (handler *Handler) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookie)
		if err != nil {
			writeProblem(w, request, http.StatusUnauthorized, "authentication_required", "Authentication required", "A valid management session is required.")
			return
		}
		if handler.services.Sessions == nil {
			handler.authenticationUnavailable(w, request)
			return
		}
		session, err := handler.services.Sessions.CurrentSession(request.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, store.ErrAuthSessionMissing) {
				handler.clearSessionCookie(w, request)
				writeProblem(w, request, http.StatusUnauthorized, "session_expired", "Session expired", "The management session is missing or expired.")
			} else {
				handler.authenticationUnavailable(w, request)
			}
			return
		}
		if webauth.ChangesState(request.Method) &&
			(!webauth.TokenEqual(request.Header.Get("X-CSRF-Token"), session.CSRFToken) || !handler.originAllowed(request)) {
			writeProblem(w, request, http.StatusForbidden, "csrf_failed", "Request rejected", "The CSRF token or request origin is invalid.")
			return
		}
		next(w, request.WithContext(context.WithValue(request.Context(), sessionContextKey{}, session)))
	}
}

func cookiePath(base string) string {
	if base == "" {
		return "/"
	}
	return base + "/"
}
