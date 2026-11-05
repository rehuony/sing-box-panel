// SPDX-License-Identifier: GPL-3.0-or-later

// Package httpapi serves the node's local console: an API on a loopback
// address through which whoever knows the console password enrolls the node
// with a panel, re-enrolls or unenrolls it, and sees its link with the panel.
// It cannot edit the node's configuration or control its runtime; the panel
// does that.
package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

const (
	sessionCookie = "sbn_session"
	// Bound JSON credentials before decoding or running password verification.
	maxLoginBody      = 4 << 10
	maxEnrollmentBody = 8 << 10
)

// Options supplies the console's collaborators.
type Options struct {
	// DataDir holds the console settings, read on every request so a
	// password change or a disabled console takes effect at once.
	DataDir string
	Link    *agent.Link
	Build   buildinfo.Info
	// Logf records enrollment changes; nil discards them.
	Logf func(format string, args ...any)
}

// Handler serves the console API.
type Handler struct {
	dataDir  string
	link     *agent.Link
	build    buildinfo.Info
	logf     func(format string, args ...any)
	now      func() time.Time
	logins   *webauth.LoginLimiter
	sessions *sessions
}

func NewHandler(options Options) *Handler {
	logf := options.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Handler{
		dataDir: options.DataDir, link: options.Link, build: options.Build, logf: logf, now: time.Now,
		logins: webauth.NewLoginLimiter(time.Now), sessions: newSessions(),
	}
}

// operation is one route of the console API. Every operation but login
// needs a session.
type operation struct {
	method string
	path   string
	public bool
	serve  func(handler *Handler, w http.ResponseWriter, request *http.Request, console settings.Console, session *session)
}

// operations lists the console API; api/node-openapi.yaml documents it.
var operations = []operation{
	{method: http.MethodGet, path: "/api/v1/auth/session", serve: (*Handler).currentSession},
	{method: http.MethodPost, path: "/api/v1/auth/session", public: true, serve: (*Handler).login},
	{method: http.MethodDelete, path: "/api/v1/auth/session", serve: (*Handler).logout},
	{method: http.MethodGet, path: "/api/v1/status", serve: (*Handler).status},
	{method: http.MethodPost, path: "/api/v1/enrollment", serve: (*Handler).enroll},
	{method: http.MethodDelete, path: "/api/v1/enrollment", serve: (*Handler).unenroll},
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	webauth.SetSecurityHeaders(w.Header())
	// A page under another name that resolves to this host must not reach
	// the console.
	if !loopbackHost(request.Host) {
		writeProblem(w, http.StatusMisdirectedRequest, "host_not_allowed", "Host not allowed", "The console answers only requests addressed to a loopback host.")
		return
	}
	var found *operation
	pathFound := false
	for index := range operations {
		if operations[index].path == request.URL.Path {
			pathFound = true
			if operations[index].method == request.Method {
				found = &operations[index]
			}
		}
	}
	switch {
	case found == nil && pathFound:
		w.Header().Set("Allow", allowedMethods(request.URL.Path))
		writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "The API operation does not support this method.")
		return
	case found == nil:
		writeProblem(w, http.StatusNotFound, "operation_not_found", "Operation not found", "The API operation does not exist.")
		return
	}
	console, err := settings.LoadConsole(handler.dataDir)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication unavailable", "The console settings cannot be read.")
		return
	}
	if !console.Enabled {
		writeProblem(w, http.StatusServiceUnavailable, "console_disabled", "Console disabled", "The console is disabled; enable it with sing-box-node console enable.")
		return
	}
	var current *session
	if !found.public {
		if current = handler.authenticate(w, request, console); current == nil {
			return
		}
	}
	found.serve(handler, w, request, console, current)
}

func allowedMethods(path string) string {
	var methods []string
	for _, operation := range operations {
		if operation.path == path {
			methods = append(methods, operation.method)
		}
	}
	return strings.Join(methods, ", ")
}

// loopbackHost reports whether a request names a loopback host.
func loopbackHost(host string) bool {
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback() && address.Zone() == ""
}

// credential identifies the console password, so changing it ends every
// session.
func credential(console settings.Console) [32]byte {
	return sha256.Sum256([]byte(console.PasswordHash))
}

func (handler *Handler) authenticate(w http.ResponseWriter, request *http.Request, console settings.Console) *session {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "authentication_required", "Authentication required", "A valid console session is required.")
		return nil
	}
	current, ok := handler.sessions.lookup(cookie.Value, credential(console), handler.now())
	if !ok {
		clearSessionCookie(w)
		writeProblem(w, http.StatusUnauthorized, "session_expired", "Session expired", "The console session is missing or expired.")
		return nil
	}
	if webauth.ChangesState(request.Method) &&
		(!webauth.TokenEqual(request.Header.Get("X-CSRF-Token"), current.csrf) || !webauth.SameOrigin(request, "")) {
		writeProblem(w, http.StatusForbidden, "csrf_failed", "Request rejected", "The CSRF token or request origin is invalid.")
		return nil
	}
	return &current
}

type sessionView struct {
	CSRFToken string    `json:"csrfToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (handler *Handler) login(w http.ResponseWriter, request *http.Request, console settings.Console, _ *session) {
	login, rejection := handler.logins.Admit(w, request, "", maxLoginBody)
	defer login.Finish()
	if rejection != nil {
		writeRejection(w, rejection)
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := jsonstrict.Decode(login.Body, maxLoginBody, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_login", "Invalid login", "The login payload is invalid.")
		return
	}
	matches, err := webauth.VerifyPassword(login.Request.Context(), input.Password, console.PasswordHash)
	switch {
	case errors.Is(err, webauth.ErrBusy):
		w.Header().Set("Retry-After", "1")
		writeProblem(w, http.StatusTooManyRequests, "login_rate_limited", "Authentication busy", "Try again shortly.")
		return
	case err != nil:
		writeRejection(w, webauth.Unavailable())
		return
	case !matches:
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Authentication failed", "The password is incorrect.")
		return
	}
	previous := ""
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		previous = cookie.Value
	}
	token, created, err := handler.sessions.create(credential(console), handler.now(), previous)
	if err != nil {
		writeRejection(w, webauth.Unavailable())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: request.TLS != nil, Expires: created.expiresAt, MaxAge: int(sessionLifetime.Seconds()),
	})
	writeJSON(w, http.StatusOK, sessionView{CSRFToken: created.csrf, ExpiresAt: created.expiresAt})
}

func (handler *Handler) currentSession(w http.ResponseWriter, _ *http.Request, _ settings.Console, current *session) {
	writeJSON(w, http.StatusOK, sessionView{CSRFToken: current.csrf, ExpiresAt: current.expiresAt})
}

func (handler *Handler) logout(w http.ResponseWriter, request *http.Request, _ settings.Console, _ *session) {
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		handler.sessions.remove(cookie.Value)
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// statusView is the node and its link with its panel.
type statusView struct {
	Node       nodeView        `json:"node"`
	Enrollment *enrollmentView `json:"enrollment"`
	Connection connectionView  `json:"connection"`
}

type nodeView struct {
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
}

type enrollmentView struct {
	PanelEndpoint string `json:"panel_endpoint"`
	PanelPath     string `json:"panel_path"`
	PanelID       string `json:"panel_id"`
	NodeID        string `json:"node_id"`
	// Trust is how the node verifies the panel: a pinned certificate key or
	// the system's certificate authorities.
	Trust string `json:"trust"`
}

type connectionView struct {
	State     agent.ConnectionState `json:"state"`
	Since     time.Time             `json:"since"`
	LastError string                `json:"last_error,omitempty"`
	RetryAt   *time.Time            `json:"retry_at,omitempty"`
}

func (handler *Handler) view(status agent.Status) statusView {
	hostname, _ := os.Hostname()
	view := statusView{
		Node: nodeView{
			Version: handler.build.DisplayVersion(), Protocol: fmt.Sprintf("%d.%d", nodeapi.ProtocolMajor, nodeapi.ProtocolMinor),
			OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: hostname,
		},
		Connection: connectionView{State: status.State, Since: status.Since.UTC(), LastError: status.LastError, RetryAt: status.RetryAt},
	}
	if view.Connection.RetryAt != nil {
		retryAt := view.Connection.RetryAt.UTC()
		view.Connection.RetryAt = &retryAt
	}
	if enrollment := status.Enrollment; enrollment != nil {
		trust := "system"
		if enrollment.Pinned {
			trust = "pin"
		}
		view.Enrollment = &enrollmentView{
			PanelEndpoint: enrollment.Endpoint, PanelPath: enrollment.Path, PanelID: enrollment.PanelID, NodeID: enrollment.NodeID, Trust: trust,
		}
	}
	return view
}

func (handler *Handler) status(w http.ResponseWriter, _ *http.Request, _ settings.Console, _ *session) {
	writeJSON(w, http.StatusOK, handler.view(handler.link.Status()))
}

func (handler *Handler) enroll(w http.ResponseWriter, request *http.Request, _ settings.Console, _ *session) {
	var input struct {
		Code string `json:"code"`
	}
	if !decodeRequest(w, request, maxEnrollmentBody, &input) {
		return
	}
	status, err := handler.link.Enroll(request.Context(), input.Code)
	var failed *agent.EnrollmentError
	switch {
	case errors.Is(err, nodeapi.ErrEnrollmentCode):
		writeProblem(w, http.StatusUnprocessableEntity, "enrollment_code_invalid", "Enrollment code invalid", "The enrollment code is malformed.")
		return
	case errors.As(err, &failed) && failed.Rejected:
		writeProblem(w, http.StatusUnprocessableEntity, "enrollment_rejected", "Enrollment rejected", failed.Reason)
		return
	case errors.As(err, &failed):
		writeProblem(w, http.StatusBadGateway, "enrollment_failed", "Enrollment failed", failed.Reason)
		return
	case err != nil:
		writeProblem(w, http.StatusInternalServerError, "enrollment_not_saved", "Enrollment not saved", err.Error())
		return
	}
	handler.logf("console: enrolled as node %s with the panel at %s", status.Enrollment.NodeID, status.Enrollment.Endpoint)
	writeJSON(w, http.StatusOK, handler.view(status))
}

func (handler *Handler) unenroll(w http.ResponseWriter, _ *http.Request, _ settings.Console, _ *session) {
	status, err := handler.link.Unenroll()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "unenrollment_failed", "Unenrollment failed", err.Error())
		return
	}
	handler.logf("console: unenrolled the node")
	writeJSON(w, http.StatusOK, handler.view(status))
}

func decodeRequest(w http.ResponseWriter, request *http.Request, limit int64, target any) bool {
	if request.Header.Get("Content-Type") != "application/json" {
		writeProblem(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "The request body must be application/json.")
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		writeProblem(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request rejected", "The request body is too large or unreadable.")
		return false
	}
	if err := jsonstrict.Decode(raw, limit, target); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_json", "Request invalid", err.Error())
		return false
	}
	return true
}

// problem is an RFC 9457 problem response.
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Type: "about:blank", Title: title, Status: status, Code: code, Detail: detail})
}

func writeRejection(w http.ResponseWriter, rejection *webauth.Rejection) {
	rejection.SetRetryAfter(w.Header())
	writeProblem(w, rejection.Status, rejection.Code, rejection.Title, rejection.Detail)
}
