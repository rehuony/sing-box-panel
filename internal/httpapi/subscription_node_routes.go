// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) createSubscriptionNode(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Outbound json.RawMessage `json:"outbound"`
	}
	if !decodeStrictRequest(w, request, store.MaximumManualNodeBytes+1024, &input) {
		return
	}
	value, err := handler.commands.CreateSubscriptionNode(request.Context(), input.Outbound)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_create_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, value)
}

func (handler *Handler) getSubscriptionNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	value, err := handler.commands.SubscriptionNode(request.Context(), id)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_read_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}

func (handler *Handler) updateSubscriptionNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input application.SubscriptionNodeWrite
	if !decodeStrictRequest(w, request, store.MaximumManualNodeBytes+1024, &input) {
		return
	}
	if input.Revision < 1 {
		writeSubscriptionInvalid(w, request)
		return
	}
	value, err := handler.commands.UpdateSubscriptionNode(request.Context(), id, input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_update_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}

func (handler *Handler) deleteSubscriptionNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Revision int64 `json:"revision"`
	}
	if !decodeStrictRequest(w, request, 1024, &input) {
		return
	}
	if err := handler.commands.DeleteSubscriptionNode(request.Context(), id, input.Revision); err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_delete_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) setSubscriptionNodeVisibility(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Hidden   *bool  `json:"hidden"`
		Revision *int64 `json:"revision"`
	}
	if !decodeStrictRequest(w, request, 1024, &input) {
		return
	}
	if input.Hidden == nil || input.Revision == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	value, err := handler.commands.SetSubscriptionNodeVisibility(request.Context(), id, *input.Hidden, *input.Revision)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_visibility_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}

func (handler *Handler) parseSubscriptionNode(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Text string `json:"text"`
	}
	if !decodeStrictRequest(w, request, store.MaximumManualNodeBytes*6+1024, &input) {
		return
	}
	value, err := handler.commands.ParseSubscriptionNode(input.Text)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_parse_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"outbound_json": string(value)})
}
