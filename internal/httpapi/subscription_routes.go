// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// The largest inner source snapshot is 4 MiB; leave bounded room for its JSON
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
	w.Header().Set("Content-Type", result.MediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
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

func matchSubscriptionRoute(path string) (resource string, identifier string, operation string, matched bool) {
	const prefix = "/api/v1/subscription/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) == 1 && validSubscriptionResource(parts[0]) {
		return parts[0], "", "", true
	}
	if len(parts) == 2 && validSubscriptionResource(parts[0]) && parts[1] != "" {
		return parts[0], parts[1], "", true
	}
	if len(parts) == 3 && parts[1] != "" {
		switch {
		case parts[0] == "channels" && parts[2] == "preview":
			return parts[0], parts[1], parts[2], true
		case parts[0] == "sources" && parts[2] == "snapshot":
			return parts[0], parts[1], parts[2], true
		case parts[0] == "tokens" && (parts[2] == "rotate" || parts[2] == "revoke"):
			return parts[0], parts[1], parts[2], true
		}
	}
	return "invalid", "", "invalid", true
}

func validSubscriptionResource(value string) bool {
	return value == "channels" || value == "sources" || value == "tokens"
}

func (handler *Handler) subscriptionManagementHandler(
	method string,
	resource string,
	identifier string,
	operation string,
) http.HandlerFunc {
	switch {
	case resource == "channels" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionChannels
	case resource == "channels" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionChannel
	case resource == "channels" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionChannel(w, request, identifier)
		}
	case resource == "channels" && identifier != "" && operation == "" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.updateSubscriptionChannel(w, request, identifier)
		}
	case resource == "channels" && identifier != "" && operation == "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.deleteSubscriptionChannel(w, request, identifier)
		}
	case resource == "channels" && identifier != "" && operation == "preview" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.previewSubscriptionChannel(w, request, identifier)
		}
	case resource == "sources" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionSources
	case resource == "sources" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionSource
	case resource == "sources" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.updateSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.deleteSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "snapshot" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.updateSubscriptionSourceSnapshot(w, request, identifier)
		}
	case resource == "tokens" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionTokens
	case resource == "tokens" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionToken
	case resource == "tokens" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionToken(w, request, identifier)
		}
	case resource == "tokens" && identifier != "" && operation == "rotate" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.rotateSubscriptionToken(w, request, identifier)
		}
	case resource == "tokens" && identifier != "" && operation == "revoke" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.revokeSubscriptionToken(w, request, identifier)
		}
	default:
		return methodNotAllowed
	}
}

func (handler *Handler) listSubscriptionChannels(w http.ResponseWriter, request *http.Request) {
	input, ok := handler.subscriptionListRequest(w, request)
	if !ok {
		return
	}
	channels, err := handler.commands.ListSubscriptionChannels(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (handler *Handler) createSubscriptionChannel(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Name    string                   `json:"name"`
		Format  store.SubscriptionFormat `json:"format"`
		Config  json.RawMessage          `json:"config"`
		Enabled *bool                    `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	channel, err := handler.commands.CreateSubscriptionChannel(request.Context(), application.CreateSubscriptionChannelRequest{
		Name: input.Name, Format: input.Format, Config: input.Config, Enabled: *input.Enabled,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_create_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusCreated, channel.UpdatedAt, channel)
}

func (handler *Handler) getSubscriptionChannel(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	channel, err := handler.commands.SubscriptionChannel(request.Context(), identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_read_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, channel.UpdatedAt, channel)
}

func (handler *Handler) updateSubscriptionChannel(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		Name    string                   `json:"name"`
		Format  store.SubscriptionFormat `json:"format"`
		Config  json.RawMessage          `json:"config"`
		Enabled *bool                    `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	channel, err := handler.commands.UpdateSubscriptionChannel(request.Context(), identifier, application.UpdateSubscriptionChannelRequest{
		Name: input.Name, Format: input.Format, Config: input.Config, Enabled: *input.Enabled, ExpectedUpdatedAt: expected,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, channel.UpdatedAt, channel)
}

func (handler *Handler) deleteSubscriptionChannel(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	if err := handler.commands.DeleteSubscriptionChannel(request.Context(), identifier, expected); err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_delete_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) previewSubscriptionChannel(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		StartupArtifactID string `json:"startup_artifact_id"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.StartupArtifactID == "" {
		writeSubscriptionInvalid(w, request)
		return
	}
	preview, err := handler.commands.RenderSubscriptionPreview(request.Context(), input.StartupArtifactID, identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler *Handler) listSubscriptionSources(w http.ResponseWriter, request *http.Request) {
	input, ok := handler.subscriptionListRequest(w, request)
	if !ok {
		return
	}
	sources, err := handler.commands.ListSubscriptionSources(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (handler *Handler) createSubscriptionSource(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Name           string                       `json:"name"`
		SourceKind     store.SubscriptionSourceKind `json:"source_kind"`
		Config         json.RawMessage              `json:"config"`
		LatestSnapshot json.RawMessage              `json:"latest_snapshot"`
		Enabled        *bool                        `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	source, err := handler.commands.CreateSubscriptionSource(request.Context(), application.CreateSubscriptionSourceRequest{
		Name: input.Name, SourceKind: input.SourceKind, Config: input.Config,
		LatestSnapshot: input.LatestSnapshot, Enabled: *input.Enabled,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_create_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusCreated, source.UpdatedAt, source)
}

func (handler *Handler) getSubscriptionSource(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	source, err := handler.commands.SubscriptionSource(request.Context(), identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_read_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, source.UpdatedAt, source)
}

func (handler *Handler) updateSubscriptionSource(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		Name       string                       `json:"name"`
		SourceKind store.SubscriptionSourceKind `json:"source_kind"`
		Config     json.RawMessage              `json:"config"`
		Enabled    *bool                        `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	source, err := handler.commands.UpdateSubscriptionSource(request.Context(), identifier, application.UpdateSubscriptionSourceRequest{
		Name: input.Name, SourceKind: input.SourceKind, Config: input.Config,
		Enabled: *input.Enabled, ExpectedUpdatedAt: expected,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, source.UpdatedAt, source)
}

func (handler *Handler) updateSubscriptionSourceSnapshot(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		LatestSnapshot json.RawMessage `json:"latest_snapshot"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if len(input.LatestSnapshot) == 0 {
		writeSubscriptionInvalid(w, request)
		return
	}
	source, err := handler.commands.UpdateSubscriptionSourceSnapshot(request.Context(), identifier, application.UpdateSubscriptionSourceSnapshotRequest{
		LatestSnapshot: input.LatestSnapshot, ExpectedUpdatedAt: expected,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_snapshot_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, source.UpdatedAt, source)
}

func (handler *Handler) deleteSubscriptionSource(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	if err := handler.commands.DeleteSubscriptionSource(request.Context(), identifier, expected); err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_delete_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) listSubscriptionTokens(w http.ResponseWriter, request *http.Request) {
	input, ok := handler.subscriptionListRequest(w, request)
	if !ok {
		return
	}
	tokens, err := handler.commands.ListSubscriptionTokens(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (handler *Handler) createSubscriptionToken(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input application.CreateSubscriptionTokenRequest
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	created, err := handler.commands.CreateSubscriptionToken(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_create_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (handler *Handler) getSubscriptionToken(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	token, err := handler.commands.SubscriptionToken(request.Context(), identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_read_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, token)
}

func (handler *Handler) rotateSubscriptionToken(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	rotation, err := handler.commands.RotateSubscriptionToken(request.Context(), identifier, input.ExpiresAt)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_rotate_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, rotation)
}

func (handler *Handler) revokeSubscriptionToken(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	token, err := handler.commands.RevokeSubscriptionToken(request.Context(), identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_revoke_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, token)
}

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
		errors.Is(err, store.ErrSubscriptionTokenNotFound), application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "subscription_resource_not_found", "Subscription resource not found", "The requested subscription resource does not exist.")
	case errors.Is(err, store.ErrSubscriptionConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "subscription_version_conflict", "Subscription version conflict", "The subscription resource changed; load its current ETag and retry.")
	case errors.Is(err, store.ErrSubscriptionChannelExists), errors.Is(err, store.ErrSubscriptionSourceExists),
		errors.Is(err, store.ErrSubscriptionTokenExists):
		writeProblem(w, request, http.StatusConflict, "subscription_resource_exists", "Subscription resource conflict", "A subscription resource with that identity already exists.")
	case errors.Is(err, store.ErrSubscriptionLimitExceeded), errors.Is(err, application.ErrSubscriptionSnapshotTooLarge):
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
