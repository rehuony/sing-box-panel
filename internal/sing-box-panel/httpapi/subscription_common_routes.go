// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

func (handler *Handler) subscriptionReadRequest(w http.ResponseWriter, request *http.Request) bool {
	_, ok := strictCoreQuery(w, request)
	return ok
}

func (handler *Handler) subscriptionListRequest(w http.ResponseWriter, request *http.Request) (subscription.SubscriptionListRequest, bool) {
	query, ok := strictCoreQuery(w, request, "limit", "before_time", "before_id")
	if !ok {
		return subscription.SubscriptionListRequest{}, false
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return subscription.SubscriptionListRequest{}, false
	}
	cursor, ok := listCursor(w, request, query.Get("before_time"), query.Get("before_id"))
	if !ok {
		return subscription.SubscriptionListRequest{}, false
	}
	return subscription.SubscriptionListRequest{Cursor: cursor, Limit: limit}, true
}

// subscriptionOrderRequest reads the IDs of a list's order to save.
func (handler *Handler) subscriptionOrderRequest(w http.ResponseWriter, request *http.Request) ([]string, bool) {
	if !handler.subscriptionMutationRequest(w, request) {
		return nil, false
	}
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeStrictRequest(w, request, 2<<20, &input) {
		return nil, false
	}
	if input.IDs == nil {
		writeSubscriptionInvalid(w, request)
		return nil, false
	}
	return input.IDs, true
}

// listCursor parses the before_time and before_id keyset cursor of a list.
func listCursor(
	w http.ResponseWriter,
	request *http.Request,
	rawTime string,
	identifier string,
) (*subscription.SubscriptionCursor, bool) {
	if rawTime == "" && identifier == "" {
		return nil, true
	}
	if rawTime == "" || identifier == "" || !store.ValidIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time and before_id must be supplied together.")
		return nil, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(rawTime))
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time must be an RFC 3339 timestamp.")
		return nil, false
	}
	return &subscription.SubscriptionCursor{CreatedAt: createdAt.UTC(), ID: identifier}, true
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
	var policy *subscription.PolicyError
	if errors.As(err, &policy) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_policy_invalid", "Channel configuration invalid", policy.Error())
		return
	}
	var parse *subscription.SourceParseError
	if errors.As(err, &parse) {
		writeSourceParseProblem(w, request, parse)
		return
	}
	var field *subscription.SourceFieldError
	if errors.As(err, &field) {
		writeProblemFields(w, request, http.StatusUnprocessableEntity, "subscription_source_invalid", "Source settings invalid",
			field.Path+": "+field.Code, map[string]string{field.Path: field.Code})
		return
	}
	var reference *store.SubscriptionReferenceError
	if errors.As(err, &reference) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_reference_invalid", "Reference invalid", reference.Path+": "+reference.Code)
		return
	}
	switch {
	case errors.Is(err, subscription.ErrInvalidSourceURL), errors.Is(err, subscription.ErrSourceAddressDenied):
		writeProblemFields(w, request, http.StatusUnprocessableEntity, "subscription_source_url_rejected", "Source URL rejected",
			sourceFetchDetail(err), map[string]string{"reason": subscription.FetchErrorCode(err)})
	case errors.Is(err, subscription.ErrSourceRequestFailed), errors.Is(err, subscription.ErrTooManySourceRedirects),
		errors.Is(err, subscription.ErrSourceBodyTooLarge):
		writeProblemFields(w, request, http.StatusBadGateway, "subscription_source_fetch_failed", "Source fetch failed",
			sourceFetchDetail(err), map[string]string{"reason": subscription.FetchErrorCode(err)})
	case errors.Is(err, subscription.ErrSourceKindMismatch):
		writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_source_kind_mismatch", "Operation not available", "Only URL sources refresh, and only file sources take uploaded content.")
	case errors.Is(err, subscription.ErrInvalidNodeDisplayName):
		writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_node_name_invalid", "Display name invalid", "A display name is 1 to 256 bytes without surrounding spaces, control characters or commas.")
	case errors.Is(err, store.ErrSubscriptionNodeNameTaken):
		writeProblem(w, request, http.StatusConflict, "subscription_node_name_taken", "Display name taken", "Another node already uses this display name.")
	case errors.Is(err, store.ErrSubscriptionRuleSetsChanged):
		writeProblem(w, request, http.StatusConflict, "subscription_rule_sets_changed", "Rule sets changed", "The client's rule sets changed since they were read; reload them and retry.")
	case errors.Is(err, store.ErrSubscriptionSourcesChanged):
		writeProblem(w, request, http.StatusConflict, "subscription_sources_changed", "Sources changed", "The sources changed since they were read; reload them and retry.")
	case errors.Is(err, store.ErrSubscriptionChannelsChanged):
		writeProblem(w, request, http.StatusConflict, "subscription_channels_changed", "Channels changed", "The channels changed since they were read; reload them and retry.")
	case errors.Is(err, store.ErrSubscriptionTokensChanged):
		writeProblem(w, request, http.StatusConflict, "subscription_tokens_changed", "Keys changed", "The keys changed since they were read; reload them and retry.")
	case errors.Is(err, store.ErrSubscriptionPoolChanged), errors.Is(err, store.ErrSubscriptionPoolInputsChanged):
		writeProblem(w, request, http.StatusConflict, "subscription_pool_changed", "Node pool changed", "The node pool changed since it was read; reload it and retry.")
	case errors.Is(err, store.ErrSubscriptionTokenSecretUnavailable):
		writeProblem(w, request, http.StatusConflict, "subscription_token_secret_unavailable", "Key cannot be revealed", "This key predates recoverable storage. Rotate it and update channel bindings to export links.")
	case errors.Is(err, store.ErrInvalidSubscriptionToken):
		writeSubscriptionInvalid(w, request)
	case errors.Is(err, store.ErrSubscriptionNodeNotFound), errors.Is(err, store.ErrSubscriptionChannelNotFound), errors.Is(err, store.ErrSubscriptionSourceNotFound),
		errors.Is(err, store.ErrSubscriptionRuleSetNotFound), errors.Is(err, store.ErrSubscriptionUserNotFound),
		errors.Is(err, store.ErrSubscriptionTokenNotFound):
		writeProblem(w, request, http.StatusNotFound, "subscription_resource_not_found", "Subscription resource not found", "The requested subscription resource does not exist.")
	case errors.Is(err, store.ErrSubscriptionConflict):
		writeProblem(w, request, http.StatusPreconditionFailed, "subscription_version_conflict", "Subscription version conflict", "The subscription resource changed; load its current ETag and retry.")
	case errors.Is(err, store.ErrSubscriptionChannelExists), errors.Is(err, store.ErrSubscriptionSourceExists),
		errors.Is(err, store.ErrSubscriptionRuleSetExists), errors.Is(err, store.ErrSubscriptionUserExists),
		errors.Is(err, store.ErrSubscriptionTokenExists):
		writeProblem(w, request, http.StatusConflict, "subscription_resource_exists", "Subscription resource conflict", "A subscription resource with that identity already exists.")
	case errors.Is(err, store.ErrSubscriptionLimitExceeded):
		writeProblem(w, request, http.StatusUnprocessableEntity, "subscription_limit_exceeded", "Subscription limit exceeded", "The subscription inputs exceed the supported count or byte budget.")
	case errors.Is(err, store.ErrSubscriptionTokenInactive):
		writeProblem(w, request, http.StatusConflict, "subscription_token_inactive", "Subscription token inactive", "The subscription token is expired or revoked.")
	default:
		// Validation errors at this boundary are intentionally not reflected: a
		// source or channel may contain credentials or other operator-only values.
		if strings.Contains(err.Error(), "subscription ") || strings.Contains(err.Error(), "invalid subscription") {
			writeSubscriptionInvalid(w, request)
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, code, "Subscription operation failed", "The subscription operation could not be completed.")
	}
}

// writeSourceParseProblem answers content that cannot be saved, naming the
// node that does not convert in the problem's fields.
func writeSourceParseProblem(w http.ResponseWriter, request *http.Request, parse *subscription.SourceParseError) {
	fields := map[string]string{"code": parse.Code}
	for name, value := range map[string]string{"location": parse.Location, "name": parse.Name, "reason": parse.Reason} {
		if value != "" {
			fields[name] = value
		}
	}
	writeProblemFields(w, request, http.StatusUnprocessableEntity, "subscription_source_unreadable", "Source content cannot be read", parse.Error(), fields)
}

// sourceFetchDetail names why a fetch failed without the URL.
func sourceFetchDetail(err error) string {
	switch {
	case errors.Is(err, subscription.ErrInvalidSourceURL):
		return "The URL must be an http or https URL with a host."
	case errors.Is(err, subscription.ErrSourceAddressDenied):
		return "The URL resolves to an address the source network policy denies."
	case errors.Is(err, subscription.ErrTooManySourceRedirects):
		return "The source redirected too many times."
	case errors.Is(err, subscription.ErrSourceBodyTooLarge):
		return "The source answered with more than 4 MiB."
	}
	return "The source could not be fetched (" + subscription.FetchErrorCode(err) + ")."
}
