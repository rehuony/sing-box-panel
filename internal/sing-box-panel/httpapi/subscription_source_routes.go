// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

// maximumSourceRequestBytes carries a source's largest content as a JSON
// string, whose escapes can grow it, and the rest of the request.
const maximumSourceRequestBytes = 2*store.MaximumSourceContentBytes + (64 << 10)

func (handler *Handler) listSubscriptionSources(w http.ResponseWriter, request *http.Request) {
	input, ok := handler.subscriptionListRequest(w, request)
	if !ok {
		return
	}
	sources, err := handler.services.Subscriptions.ListSubscriptionSources(request.Context(), input)
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
	var input subscription.CreateSubscriptionSourceRequest
	if !decodeStrictRequest(w, request, maximumSourceRequestBytes, &input) {
		return
	}
	source, err := handler.services.Subscriptions.CreateSubscriptionSource(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_create_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusCreated, source.UpdatedAt, source)
}

func (handler *Handler) previewSubscriptionSource(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input subscription.SubscriptionSourcePreviewRequest
	if !decodeStrictRequest(w, request, maximumSourceRequestBytes, &input) {
		return
	}
	preview, err := handler.services.Subscriptions.PreviewSubscriptionSource(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler *Handler) getSubscriptionSource(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	source, err := handler.services.Subscriptions.SubscriptionSource(request.Context(), identifier)
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
	var input subscription.UpdateSubscriptionSourceRequest
	if !decodeStrictRequest(w, request, maximumSourceRequestBytes, &input) {
		return
	}
	source, err := handler.services.Subscriptions.UpdateSubscriptionSource(request.Context(), identifier, input, expected)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, source.UpdatedAt, source)
}

func (handler *Handler) refreshSubscriptionSource(w http.ResponseWriter, request *http.Request, sourceID string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	source, err := handler.services.Subscriptions.RefreshSubscriptionSource(request.Context(), sourceID)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_refresh_failed", err)
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
	if err := handler.services.Subscriptions.DeleteSubscriptionSource(request.Context(), identifier, expected); err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_delete_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
