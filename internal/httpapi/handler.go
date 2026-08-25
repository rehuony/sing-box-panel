// SPDX-License-Identifier: GPL-3.0-or-later

// Package httpapi implements the versioned management and public HTTP boundary.
package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

const (
	maxLoginBody          = 8 << 10
	maxCanonicalPatchBody = 5 << 20
	sessionCookie         = "sbp_session"
)

type StatusProvider interface {
	SystemStatus(context.Context) (SystemStatus, error)
	DashboardContext(context.Context) (DashboardContext, error)
}

type SystemStatus struct {
	PanelVersion      string  `json:"panel_version"`
	CanonicalRevision int64   `json:"canonical_revision"`
	AppliedBundleID   *string `json:"applied_bundle_id"`
	Running           bool    `json:"running"`
	RunningVersion    *string `json:"running_version"`
	RunningArtifact   *string `json:"running_artifact"`
	CapabilityState   string  `json:"capability_state"`
}

type DashboardContext struct {
	View       DashboardView       `json:"view"`
	Running    *DashboardRuntime   `json:"running"`
	Canonical  DashboardCanonical  `json:"canonical"`
	Applied    *DashboardApplied   `json:"applied"`
	Capability DashboardCapability `json:"capability"`
}

type DashboardView struct {
	ExactVersion string `json:"exactVersion"`
}

type DashboardRuntime struct {
	ExactVersion string `json:"exactVersion"`
	ArtifactName string `json:"artifactName"`
	Digest       string `json:"digest"`
}

type DashboardCanonical struct {
	Revision            int64     `json:"revision"`
	SavedAt             time.Time `json:"savedAt"`
	HasUnappliedChanges bool      `json:"hasUnappliedChanges"`
}

type DashboardApplied struct {
	Bundle    string    `json:"bundle"`
	Revision  int64     `json:"revision"`
	AppliedAt time.Time `json:"appliedAt"`
}

type DashboardCapability struct {
	Level   string  `json:"level"`
	Label   string  `json:"label"`
	Warning *string `json:"warning"`
}

type HandlerOptions struct {
	Settings settings.Settings
	Build    buildinfo.Info
	Assets   fs.FS
	Status   StatusProvider
	Commands *application.Application
}

type Handler struct {
	settings settings.Settings
	build    buildinfo.Info
	sessions *sessions
	logins   *loginLimiter
	status   StatusProvider
	commands *application.Application
	assetFS  fs.FS
	assets   http.Handler
}

func NewHandler(options HandlerOptions) *Handler {
	var assets http.Handler
	if options.Assets != nil {
		assets = http.FileServer(http.FS(options.Assets))
	}
	return &Handler{
		settings: options.Settings,
		build:    options.Build,
		sessions: newSessions(),
		logins:   newLoginLimiter(),
		status:   options.Status,
		commands: options.Commands,
		assetFS:  options.Assets,
		assets:   assets,
	}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	requestID := request.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = newRequestID()
		request.Header.Set("X-Request-ID", requestID)
	}
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
	if handler.settings.Server.BasePath != "" && request.Method == http.MethodGet && request.URL.Path == handler.settings.Server.BasePath {
		target := handler.settings.Server.BasePath + "/"
		if request.URL.RawQuery != "" {
			target += "?" + request.URL.RawQuery
		}
		http.Redirect(w, request, target, http.StatusPermanentRedirect)
		return
	}

	path, ok := handler.stripBasePath(request.URL.Path)
	if !ok {
		http.NotFound(w, request)
		return
	}
	switch {
	case request.Method == http.MethodGet && path == "/api/v1/health":
		handler.health(w)
	case path == "/sub" || strings.HasPrefix(path, "/sub/"):
		handler.publicSubscription(w, request, path)
	case request.Method == http.MethodGet && path == "/api/v1/auth/session":
		handler.authenticated(handler.currentSession)(w, request)
	case request.Method == http.MethodPost && path == "/api/v1/auth/session":
		handler.login(w, request)
	case request.Method == http.MethodDelete && path == "/api/v1/auth/session":
		handler.authenticated(handler.logout)(w, request)
	case request.Method == http.MethodGet && path == "/api/v1/system/status":
		handler.authenticated(handler.systemStatus)(w, request)
	case request.Method == http.MethodGet && path == "/api/v1/dashboard/context":
		handler.authenticated(handler.dashboardContext)(w, request)
	case request.Method == http.MethodGet && path == "/api/v1/config/canonical":
		handler.authenticated(handler.canonicalDocument)(w, request)
	case request.Method == http.MethodPut && path == "/api/v1/config/canonical":
		handler.authenticated(handler.replaceCanonicalDocument)(w, request)
	case request.Method == http.MethodPatch && path == "/api/v1/config/canonical":
		handler.authenticated(handler.patchCanonicalDocument)(w, request)
	case handler.handleApplicationRoute(w, request, path):
		return
	case strings.HasPrefix(path, "/api/"):
		writeProblem(w, request, http.StatusNotFound, "operation_not_found", "Operation not found", "The API operation does not exist.")
	case handler.assets != nil && request.Method == http.MethodGet:
		handler.serveAsset(w, request, path)
	default:
		http.NotFound(w, request)
	}
}

func (handler *Handler) stripBasePath(path string) (string, bool) {
	base := handler.settings.Server.BasePath
	if base == "" {
		return path, true
	}
	if path == base {
		return "/", true
	}
	if !strings.HasPrefix(path, base+"/") {
		return "", false
	}
	return strings.TrimPrefix(path, base), true
}

func (handler *Handler) health(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": handler.build.Version})
}

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
	writeJSON(w, http.StatusOK, sessionPayload{
		DisplayName: "Administrator",
		CSRFToken:   csrf,
		ExpiresAt:   expiresAt.UTC(),
	})
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
	writeJSON(w, http.StatusOK, sessionPayload{
		DisplayName: "Administrator",
		CSRFToken:   current.csrf,
		ExpiresAt:   current.expiresAt.UTC(),
	})
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

func (handler *Handler) systemStatus(w http.ResponseWriter, request *http.Request) {
	if handler.status == nil {
		writeJSON(w, http.StatusOK, SystemStatus{PanelVersion: handler.build.Version, CapabilityState: "unavailable"})
		return
	}
	status, err := handler.status.SystemStatus(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "status_unavailable", "Status unavailable", "The current system status could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler *Handler) dashboardContext(w http.ResponseWriter, request *http.Request) {
	if handler.status == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "dashboard_unavailable", "Dashboard unavailable", "The control-plane context is not ready.")
		return
	}
	context, err := handler.status.DashboardContext(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "dashboard_unavailable", "Dashboard unavailable", "The control-plane context could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, context)
}

func (handler *Handler) canonicalDocument(w http.ResponseWriter, request *http.Request) {
	if handler.commands == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Canonical configuration services are not ready.")
		return
	}
	head, err := handler.commands.CanonicalHead(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "canonical_read_failed", "Configuration unavailable", "The canonical revision could not be loaded.")
		return
	}
	if head == nil {
		writeProblem(w, request, http.StatusNotFound, "canonical_not_initialized", "Configuration not initialized", "No canonical revision has been saved.")
		return
	}
	w.Header().Set("ETag", quoteETag(head.ID))
	writeJSON(w, http.StatusOK, head)
}

func (handler *Handler) replaceCanonicalDocument(w http.ResponseWriter, request *http.Request) {
	if handler.commands == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Canonical configuration services are not ready.")
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	raw, err := readBoundedBody(request, canonical.MaximumBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "canonical_too_large", "Configuration rejected", err.Error())
		return
	}
	result, err := handler.commands.ReplaceCanonical(request.Context(), expectedHead, raw)
	if err != nil {
		switch {
		case application.IsRevisionConflict(err):
			writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
		case errors.Is(err, canonical.ErrInvalidDocument):
			writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_invalid", "Configuration invalid", err.Error())
		default:
			writeProblem(w, request, http.StatusInternalServerError, "canonical_save_failed", "Configuration save failed", "The canonical revision could not be saved.")
		}
		return
	}
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) patchCanonicalDocument(w http.ResponseWriter, request *http.Request) {
	if handler.commands == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Canonical configuration services are not ready.")
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	raw, err := readBoundedBody(request, maxCanonicalPatchBody)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "canonical_patch_too_large", "Configuration patch rejected", err.Error())
		return
	}
	var input struct {
		Changes []application.CanonicalChange `json:"changes"`
	}
	if err := jsonstrict.Decode(raw, maxCanonicalPatchBody, &input); err != nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_patch_invalid", "Configuration patch invalid", err.Error())
		return
	}
	result, err := handler.commands.PatchCanonical(request.Context(), expectedHead, input.Changes)
	if err != nil {
		switch {
		case application.IsRevisionConflict(err):
			writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
		case errors.Is(err, application.ErrCanonicalPatchInvalid):
			writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_patch_invalid", "Configuration patch invalid", err.Error())
		default:
			writeProblem(w, request, http.StatusInternalServerError, "canonical_patch_failed", "Configuration patch failed", "The canonical changes could not be saved.")
		}
		return
	}
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, http.StatusOK, result)
}

func parseIfMatch(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' || strings.HasPrefix(value, "W/") {
		return "", errors.New(`If-Match must contain one quoted revision ID, or "none" for the first revision`)
	}
	identifier := value[1 : len(value)-1]
	if identifier == "none" {
		return "", nil
	}
	if len(identifier) > 256 || identifier == "" || strings.ContainsAny(identifier, "\"\\,\r\n") {
		return "", errors.New("If-Match contains an invalid revision ID")
	}
	return identifier, nil
}

func quoteETag(identifier string) string {
	return `"` + identifier + `"`
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
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+request.Host
}

func (handler *Handler) serveAsset(w http.ResponseWriter, request *http.Request, path string) {
	clone := request.Clone(request.Context())
	assetPath := strings.TrimPrefix(path, "/")
	if assetPath == "" {
		assetPath = "index.html"
	}
	if info, err := fs.Stat(handler.assetFS, assetPath); err != nil || info.IsDir() {
		assetPath = "index.html"
	}
	if assetPath == "index.html" {
		handler.serveIndex(w, request)
		return
	}
	clone.URL.Path = "/" + assetPath
	handler.assets.ServeHTTP(w, clone)
}

func (handler *Handler) serveIndex(w http.ResponseWriter, request *http.Request) {
	data, err := fs.ReadFile(handler.assetFS, "index.html")
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "frontend_unavailable", "Frontend unavailable", "The embedded frontend index is missing.")
		return
	}
	data = bytes.ReplaceAll(data, []byte("__SBP_BASE_PATH__"), []byte(handler.settings.Server.BasePath))
	baseHref := handler.settings.Server.BasePath + "/"
	if baseHref == "" {
		baseHref = "/"
	}
	data = bytes.ReplaceAll(data, []byte(`<base href="/" data-sbp-runtime />`), []byte(`<base href="`+baseHref+`" data-sbp-runtime />`))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func cookiePath(base string) string {
	if base == "" {
		return "/"
	}
	return base + "/"
}

func readBoundedBody(request *http.Request, limit int64) ([]byte, error) {
	defer request.Body.Close()
	data, err := io.ReadAll(io.LimitReader(request.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("request body is too large")
	}
	return data, nil
}

func newRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(raw)
}
