// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

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
		Name       string                   `json:"name"`
		Format     store.SubscriptionFormat `json:"format"`
		PublicHost string                   `json:"public_host"`
		Config     json.RawMessage          `json:"config"`
		Enabled    *bool                    `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	channel, err := handler.commands.CreateSubscriptionChannel(request.Context(), application.CreateSubscriptionChannelRequest{
		Name: input.Name, Format: input.Format, PublicHost: input.PublicHost,
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
		Name       string                   `json:"name"`
		Format     store.SubscriptionFormat `json:"format"`
		PublicHost string                   `json:"public_host"`
		Config     json.RawMessage          `json:"config"`
		Enabled    *bool                    `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	channel, err := handler.commands.UpdateSubscriptionChannel(request.Context(), identifier, application.UpdateSubscriptionChannelRequest{
		Name: input.Name, Format: input.Format, PublicHost: input.PublicHost,
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
		UserID string `json:"user_id"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.UserID == "" {
		writeSubscriptionInvalid(w, request)
		return
	}
	preview, err := handler.commands.RenderSubscriptionPreview(request.Context(), input.UserID, identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
