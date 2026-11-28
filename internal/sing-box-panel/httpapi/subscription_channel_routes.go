// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

func (handler *Handler) listSubscriptionChannels(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	channels, err := handler.services.Subscriptions.ListSubscriptionChannels(request.Context())
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (handler *Handler) setSubscriptionChannelOrder(w http.ResponseWriter, request *http.Request) {
	ids, ok := handler.subscriptionOrderRequest(w, request)
	if !ok {
		return
	}
	if err := handler.services.Subscriptions.SetSubscriptionChannelOrder(request.Context(), ids); err != nil {
		writeSubscriptionProblem(w, request, "subscription_channel_order_failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	channel, err := handler.services.Subscriptions.CreateSubscriptionChannel(request.Context(), subscription.CreateSubscriptionChannelRequest{
		Name: input.Name, Format: input.Format,
		Config: input.Config, Enabled: *input.Enabled,
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
	channel, err := handler.services.Subscriptions.SubscriptionChannel(request.Context(), identifier)
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
	channel, err := handler.services.Subscriptions.UpdateSubscriptionChannel(request.Context(), identifier, subscription.UpdateSubscriptionChannelRequest{
		Name: input.Name, Format: input.Format,
		Config: input.Config, Enabled: *input.Enabled, ExpectedUpdatedAt: expected,
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
	if err := handler.services.Subscriptions.DeleteSubscriptionChannel(request.Context(), identifier, expected); err != nil {
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
		UserID string                                 `json:"user_id"`
		Draft  *subscription.SubscriptionDraftPreview `json:"draft,omitempty"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	preview, err := handler.services.Subscriptions.RenderSubscriptionDraft(request.Context(), input.UserID, identifier, input.Draft)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
