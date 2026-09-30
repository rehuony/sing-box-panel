// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type sessionContextKey struct{}
type sessionPayload = application.AuthSession

const loginFingerprintHeader = "X-Client-Fingerprint"
const loginTimeout = 5 * time.Second

var loginFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func writeLoginRateLimit(w http.ResponseWriter, request *http.Request, retryAfter time.Duration) {
	seconds := max(1, int((retryAfter+time.Second-1)/time.Second))
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeProblem(w, request, http.StatusTooManyRequests, "login_rate_limited", "Too many attempts", "Wait before trying to authenticate again.")
}

func (handler *Handler) allowLogin(w http.ResponseWriter, request *http.Request) bool {
	if allowed, retryAfter := handler.logins.begin(loginClient(request.RemoteAddr)); !allowed {
		writeLoginRateLimit(w, request, retryAfter)
		return false
	}
	return true
}

func (handler *Handler) login(w http.ResponseWriter, request *http.Request) {
	controller := http.NewResponseController(w)
	// A rejected HTTP/1 login must not keep the connection busy draining an
	// attacker-controlled body after the handler has released its work slot.
	if request.ProtoMajor == 1 {
		w.Header().Set("Connection", "close")
		request.Close = true
		defer func() {
			_ = controller.SetReadDeadline(time.Now())
			_ = request.Body.Close()
		}()
	}
	// Bound response writes before admission so rejected logins are protected
	// too. Keep the deadline armed for the server's post-handler buffered flush.
	ctx, cancel := context.WithTimeout(request.Context(), loginTimeout)
	defer cancel()
	request = request.WithContext(ctx)
	deadline, _ := ctx.Deadline()
	if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		handler.authenticationUnavailable(w, request)
		return
	}
	if !handler.allowLogin(w, request) {
		return
	}
	defer handler.logins.finish()
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		handler.authenticationUnavailable(w, request)
		return
	}
	fingerprint := request.Header.Get(loginFingerprintHeader)
	if values := request.Header.Values(loginFingerprintHeader); len(values) > 1 || (len(values) == 1 && !loginFingerprintPattern.MatchString(fingerprint)) {
		writeProblem(w, request, http.StatusBadRequest, "invalid_login", "Invalid login", "The login fingerprint is invalid.")
		return
	}
	if allowed, retryAfter := handler.logins.fingerprint(fingerprint); !allowed {
		writeLoginRateLimit(w, request, retryAfter)
		return
	}
	if !handler.sameOrigin(request) || request.Header.Get("Content-Type") != "application/json" {
		writeProblem(w, request, http.StatusForbidden, "csrf_failed", "Request rejected", "A same-origin JSON login request is required.")
		return
	}
	if request.ContentLength > maxLoginBody {
		writeProblem(w, request, http.StatusBadRequest, "invalid_body", "Invalid request", "The login payload is invalid.")
		return
	}
	data, err := readBoundedBody(request, maxLoginBody)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "invalid_body", "Invalid request", "The login payload is invalid.")
		return
	}
	var input application.LoginInput
	if err := jsonstrict.Decode(data, maxLoginBody, &input); err != nil {
		writeProblem(w, request, http.StatusBadRequest, "invalid_login", "Invalid login", "The login payload is invalid.")
		return
	}
	if handler.commands == nil {
		handler.authenticationUnavailable(w, request)
		return
	}
	previous, _ := request.Cookie(sessionCookie)
	previousToken := ""
	if previous != nil {
		previousToken = previous.Value
	}
	value, err := handler.commands.Login(request.Context(), input, previousToken)
	if err != nil {
		if errors.Is(err, application.ErrInvalidCredentials) {
			writeProblem(w, request, http.StatusUnauthorized, "invalid_credentials", "Authentication failed", "The email or password is incorrect.")
		} else if errors.Is(err, auth.ErrBusy) {
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
		Expires: value.ExpiresAt, MaxAge: int(application.SessionLifetime.Seconds()),
	}
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusOK, value)
}

func (handler *Handler) currentSession(w http.ResponseWriter, request *http.Request) {
	value, ok := request.Context().Value(sessionContextKey{}).(application.AuthSession)
	if !ok {
		handler.authenticationUnavailable(w, request)
		return
	}
	writeJSON(w, http.StatusOK, value)
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
	if err := handler.commands.Logout(request.Context(), cookie.Value); err != nil {
		handler.authenticationUnavailable(w, request)
		return
	}
	handler.clearSessionCookie(w, request)
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) authenticationUnavailable(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication unavailable", "The authentication service is temporarily unavailable.")
}

func (handler *Handler) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookie)
		if err != nil {
			writeProblem(w, request, http.StatusUnauthorized, "authentication_required", "Authentication required", "A valid management session is required.")
			return
		}
		if handler.commands == nil {
			handler.authenticationUnavailable(w, request)
			return
		}
		session, err := handler.commands.CurrentSession(request.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, store.ErrAuthSessionMissing) {
				handler.clearSessionCookie(w, request)
				writeProblem(w, request, http.StatusUnauthorized, "session_expired", "Session expired", "The management session is missing or expired.")
			} else {
				handler.authenticationUnavailable(w, request)
			}
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions {
			if !constantTimeTokenEqual(request.Header.Get("X-CSRF-Token"), session.CSRFToken) || !handler.sameOrigin(request) {
				writeProblem(w, request, http.StatusForbidden, "csrf_failed", "Request rejected", "The CSRF token or request origin is invalid.")
				return
			}
		}
		next(w, request.WithContext(context.WithValue(request.Context(), sessionContextKey{}, session)))
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
