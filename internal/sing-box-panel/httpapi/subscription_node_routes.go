// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
)

func (handler *Handler) listSubscriptionNodes(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	list, err := handler.services.Subscriptions.SubscriptionNodes(request.Context())
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (handler *Handler) getSubscriptionNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	node, err := handler.services.Subscriptions.SubscriptionNode(request.Context(), id)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_read_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, node.UpdatedAt, node)
}

func (handler *Handler) updateSubscriptionNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		DisplayName *string `json:"display_name"`
	}
	if !decodeStrictRequest(w, request, 4096, &input) {
		return
	}
	if input.DisplayName == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	node, err := handler.services.Subscriptions.UpdateSubscriptionNode(request.Context(), id, *input.DisplayName, expected)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, node.UpdatedAt, node)
}

func (handler *Handler) setSubscriptionNodeOrder(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeStrictRequest(w, request, 2<<20, &input) {
		return
	}
	if input.IDs == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	list, err := handler.services.Subscriptions.SetSubscriptionNodeOrder(request.Context(), input.IDs)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_order_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
