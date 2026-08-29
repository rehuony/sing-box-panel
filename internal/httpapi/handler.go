// SPDX-License-Identifier: GPL-3.0-or-later

// Package httpapi implements the versioned management and public HTTP boundary.
package httpapi

import (
	"context"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
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
	PanelVersion       string  `json:"panel_version"`
	CanonicalRevision  int64   `json:"canonical_revision"`
	AppliedBundleID    *string `json:"applied_bundle_id"`
	Running            bool    `json:"running"`
	RunningVersion     *string `json:"running_version"`
	RunningArtifact    *string `json:"running_artifact"`
	ConfigurationState string  `json:"configuration_state"`
}

type DashboardContext struct {
	View      DashboardView      `json:"view"`
	Running   *DashboardRuntime  `json:"running"`
	Canonical DashboardCanonical `json:"canonical"`
	Applied   *DashboardApplied  `json:"applied"`
	Adapter   DashboardAdapter   `json:"adapter"`
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

type DashboardAdapter struct {
	Supported bool    `json:"supported"`
	Label     string  `json:"label"`
	Warning   *string `json:"warning"`
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
	if strings.HasPrefix(path, "/api/v1/") {
		_, methodAllowed, pathRegistered := registeredManagementOperation(request.Method, path)
		if !pathRegistered {
			writeProblem(w, request, http.StatusNotFound, "operation_not_found", "Operation not found", "The API operation does not exist.")
			return
		}
		if !methodAllowed {
			methodNotAllowed(w, request)
			return
		}
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
