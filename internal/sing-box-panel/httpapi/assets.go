// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

var hashedFrontendAsset = regexp.MustCompile(`^assets/[^/]+-[A-Za-z0-9_-]{8}\.(js|css)$`)

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
	if hashedFrontendAsset.MatchString(assetPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	handler.assets.ServeHTTP(w, clone)
}

func (handler *Handler) serveIndex(w http.ResponseWriter, request *http.Request) {
	data, err := fs.ReadFile(handler.assetFS, "index.html")
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "frontend_unavailable", "Frontend unavailable", "The embedded frontend index is missing.")
		return
	}
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "frontend_unavailable", "Frontend unavailable", "Unable to initialize frontend style policy.")
		return
	}
	nonce := hex.EncodeToString(nonceBytes)
	data = bytes.ReplaceAll(data, []byte("__SBP_STYLE_NONCE__"), []byte(nonce))
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'nonce-"+nonce+"'; img-src 'self' data:; connect-src 'self'")
	data = bytes.ReplaceAll(data, []byte("__SBP_BASE_PATH__"), []byte(handler.settings.Server.BasePath))
	appearance := handler.settings.Panel.Appearance
	if handler.services.Settings != nil {
		if saved, err := handler.services.Settings.PanelAppearance(request.Context()); err == nil {
			appearance = saved
		}
	}
	// Only visual preferences belong in the anonymous HTML, never panel settings.
	// Keep login available with the startup appearance if settings cannot be read.
	appearanceJSON, err := json.Marshal(appearance)
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "frontend_unavailable", "Frontend unavailable", "Unable to initialize frontend appearance.")
		return
	}
	data = bytes.ReplaceAll(data, []byte("__SBP_APPEARANCE__"), []byte(html.EscapeString(string(appearanceJSON))))
	data = bytes.ReplaceAll(data, []byte("/*__SBP_APPEARANCE_CSS__*/"), []byte(initialAppearanceCSS(appearance)))
	baseHref := handler.settings.Server.BasePath + "/"
	if baseHref == "" {
		baseHref = "/"
	}
	data = bytes.ReplaceAll(data, []byte(`<base href="/" data-sbp-runtime />`), []byte(`<base href="`+baseHref+`" data-sbp-runtime />`))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// errBodyTooLarge reports a request body over its operation's limit; any
// other error from readBoundedBody is a failed read, such as a client that
// stopped sending.
var errBodyTooLarge = errors.New("request body is too large")

func readBoundedBody(request *http.Request, limit int64) ([]byte, error) {
	defer request.Body.Close()
	data, err := io.ReadAll(io.LimitReader(request.Body, limit+1))
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytes):
		return nil, errBodyTooLarge
	case err != nil:
		return nil, fmt.Errorf("read request body: %w", err)
	case int64(len(data)) > limit:
		return nil, errBodyTooLarge
	}
	return data, nil
}

// writeBodyProblem answers a request body readBoundedBody refused.
func writeBodyProblem(w http.ResponseWriter, request *http.Request, err error) {
	if errors.Is(err, errBodyTooLarge) {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "request_too_large", "Request rejected", "The request body is too large.")
		return
	}
	writeProblem(w, request, http.StatusBadRequest, "request_body_unreadable", "Request invalid", "The request body could not be read.")
}

func newRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(raw)
}
