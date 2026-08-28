// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) subscriptionReadRequest(w http.ResponseWriter, request *http.Request) bool {
	if !handler.requireCommands(w, request) {
		return false
	}
	_, ok := strictCoreQuery(w, request)
	return ok
}

func (handler *Handler) subscriptionListRequest(
	w http.ResponseWriter,
	request *http.Request,
) (application.SubscriptionListRequest, bool) {
	if !handler.requireCommands(w, request) {
		return application.SubscriptionListRequest{}, false
	}
	query, ok := strictCoreQuery(w, request, "limit", "before_time", "before_id")
	if !ok {
		return application.SubscriptionListRequest{}, false
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return application.SubscriptionListRequest{}, false
	}
	cursor, ok := startupArtifactCursor(w, request, query.Get("before_time"), query.Get("before_id"))
	if !ok {
		return application.SubscriptionListRequest{}, false
	}
	var subscriptionCursor *application.SubscriptionCursor
	if cursor != nil {
		subscriptionCursor = &application.SubscriptionCursor{CreatedAt: cursor.CreatedAt, ID: cursor.ID}
	}
	return application.SubscriptionListRequest{Cursor: subscriptionCursor, Limit: limit}, true
}

func (handler *Handler) subscriptionMutationRequest(w http.ResponseWriter, request *http.Request) bool {
	return handler.subscriptionReadRequest(w, request)
}

func subscriptionIfMatch(w http.ResponseWriter, request *http.Request) (time.Time, bool) {
	raw, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil || raw == "" {
		writeProblem(w, request, http.StatusPreconditionRequired, "subscription_version_required", "Subscription version required", "If-Match must contain the quoted current updated_at value.")
		return time.Time{}, false
	}
	expected, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "subscription_version_required", "Subscription version required", "If-Match must contain the quoted current updated_at value.")
		return time.Time{}, false
	}
	return expected.UTC(), true
}

func subscriptionETag(updatedAt time.Time) string {
	return quoteETag(updatedAt.UTC().Format(time.RFC3339Nano))
}

func writeSubscriptionResource(w http.ResponseWriter, status int, updatedAt time.Time, value any) {
	w.Header().Set("ETag", subscriptionETag(updatedAt))
	writeJSON(w, status, value)
}

func writeSubscriptionInvalid(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_invalid", "Subscription request invalid", "The subscription request is incomplete or invalid.")
}

func writeSubscriptionProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case errors.Is(err, store.ErrSubscriptionChannelNotFound), errors.Is(err, store.ErrSubscriptionSourceNotFound),
		errors.Is(err, store.ErrSubscriptionSourceVersionNotFound),
		errors.Is(err, store.ErrSubscriptionUserNotFound),
		errors.Is(err, store.ErrSubscriptionTokenNotFound), application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "subscription_resource_not_found", "Subscription resource not found", "The requested subscription resource does not exist.")
	case errors.Is(err, store.ErrSubscriptionConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "subscription_version_conflict", "Subscription version conflict", "The subscription resource changed; load its current ETag and retry.")
	case errors.Is(err, store.ErrSubscriptionChannelExists), errors.Is(err, store.ErrSubscriptionSourceExists),
		errors.Is(err, store.ErrSubscriptionUserExists),
		errors.Is(err, store.ErrSubscriptionTokenExists):
		writeProblem(w, request, http.StatusConflict, "subscription_resource_exists", "Subscription resource conflict", "A subscription resource with that identity already exists.")
	case errors.Is(err, store.ErrSubscriptionLimitExceeded):
		writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_limit_exceeded", "Subscription limit exceeded", "The enabled subscription inputs exceed the supported count or byte budget.")
	case errors.Is(err, store.ErrSubscriptionTokenInactive):
		writeProblem(w, request, http.StatusConflict, "subscription_token_inactive", "Subscription token inactive", "The subscription token is expired or revoked.")
	case errors.Is(err, application.ErrSubscriptionPreviewArtifactState):
		writeProblem(w, request, http.StatusConflict, "subscription_preview_unavailable", "Subscription preview unavailable", "The startup artifact is not ready or stale.")
	default:
		// Validation errors at this boundary are intentionally not reflected: a
		// source config may contain credentials or other operator-only values.
		if strings.Contains(err.Error(), "subscription ") || strings.Contains(err.Error(), "invalid subscription") {
			writeSubscriptionInvalid(w, request)
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, code, "Subscription operation failed", "The subscription operation could not be completed.")
	}
}
