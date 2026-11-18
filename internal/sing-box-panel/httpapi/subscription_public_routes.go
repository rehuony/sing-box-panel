// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

// The largest inner source version is 4 MiB; leave bounded room for its JSON
// envelope without weakening the store's stricter per-field limits.
const maximumSubscriptionRequestBytes = (4 << 20) + (64 << 10)

// maximumSourceVersionRequestBytes carries a source version of the largest
// size a fetch accepts as base64 text, which is a third larger, and its
// envelope.
const maximumSourceVersionRequestBytes = (subscription.MaximumSourceBodyBytes+2)/3*4 + (64 << 10)

func (handler *Handler) publicSubscription(w http.ResponseWriter, request *http.Request, path string) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, request)
		return
	}
	if handler.services.Subscriptions == nil {
		writePublicSubscriptionProblem(w, request, errPublicSubscriptionUnavailable)
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/sub/"), "/")
	if len(parts) != 2 || parts[0] == "" || len(parts[0]) > 512 ||
		!store.ValidIdentifier(parts[1]) || len(parts[1]) > 128 {
		writePublicSubscriptionProblem(w, request, subscription.ErrPublicSubscriptionAccessDenied)
		return
	}
	result, err := handler.services.Subscriptions.PublicSubscription(request.Context(), parts[0], parts[1])
	if err != nil {
		writePublicSubscriptionProblem(w, request, err)
		return
	}
	etag := quoteETag(result.ETag)
	notModified := noneMatchHits(request.Header.Get("If-None-Match"), etag)
	bodyBytes := int64(len(result.Body))
	if notModified {
		bodyBytes = 0
	}
	if err := handler.services.Subscriptions.RecordPublicSubscriptionUse(request.Context(), result.TokenID, bodyBytes); err != nil {
		writePublicSubscriptionProblem(w, request, err)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d",
		result.Traffic.UploadBytes, result.Traffic.DownloadBytes, result.Traffic.TotalBytes))
	if notModified {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", result.MediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
}

var errPublicSubscriptionUnavailable = errors.New("public subscription service unavailable")

func writePublicSubscriptionProblem(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNoAppliedBundle):
		writeProblem(w, request, http.StatusServiceUnavailable, "not_applied", "Subscription unavailable", "No applied subscription bundle is available.")
	case errors.Is(err, subscription.ErrPublicSubscriptionAccessDenied),
		errors.Is(err, subscription.ErrPublicSubscriptionChannelUnavailable):
		writeProblem(w, request, http.StatusNotFound, "subscription_not_found", "Subscription not found", "The requested subscription is unavailable.")
	default:
		writeProblem(w, request, http.StatusServiceUnavailable, "subscription_unavailable", "Subscription unavailable", "The subscription could not be served.")
	}
}
