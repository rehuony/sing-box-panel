// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// The largest inner source version is 4 MiB; leave bounded room for its JSON
// envelope without weakening the store's stricter per-field limits.
const maximumSubscriptionRequestBytes = (4 << 20) + (64 << 10)

func (handler *Handler) publicSubscription(w http.ResponseWriter, request *http.Request, path string) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, request)
		return
	}
	if handler.commands == nil {
		writePublicSubscriptionProblem(w, request, errPublicSubscriptionUnavailable)
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/sub/"), "/")
	if len(parts) != 2 || parts[0] == "" || len(parts[0]) > 512 ||
		!validStableIdentifier(parts[1]) || len(parts[1]) > 128 {
		writePublicSubscriptionProblem(w, request, application.ErrPublicSubscriptionAccessDenied)
		return
	}
	result, err := handler.commands.PublicSubscription(request.Context(), parts[0], parts[1])
	if err != nil {
		writePublicSubscriptionProblem(w, request, err)
		return
	}
	etag := quoteETag(result.ETag)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	if publicSubscriptionETagMatches(request.Header.Get("If-None-Match"), etag) {
		if err := handler.commands.RecordPublicSubscriptionUse(request.Context(), result.TokenID, 0); err != nil {
			writePublicSubscriptionProblem(w, request, err)
			return
		}
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if err := handler.commands.RecordPublicSubscriptionUse(request.Context(), result.TokenID, int64(len(result.Body))); err != nil {
		writePublicSubscriptionProblem(w, request, err)
		return
	}
	w.Header().Set("Content-Type", result.MediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
}

func publicSubscriptionETagMatches(header string, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == current {
			return true
		}
	}
	return false
}

var errPublicSubscriptionUnavailable = errors.New("public subscription service unavailable")

func writePublicSubscriptionProblem(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNoAppliedBundle):
		writeProblem(w, request, http.StatusServiceUnavailable, "not_applied", "Subscription unavailable", "No applied subscription bundle is available.")
	case errors.Is(err, application.ErrPublicSubscriptionAccessDenied),
		errors.Is(err, application.ErrPublicSubscriptionChannelUnavailable):
		writeProblem(w, request, http.StatusNotFound, "subscription_not_found", "Subscription not found", "The requested subscription is unavailable.")
	default:
		writeProblem(w, request, http.StatusServiceUnavailable, "subscription_unavailable", "Subscription unavailable", "The subscription could not be served.")
	}
}
