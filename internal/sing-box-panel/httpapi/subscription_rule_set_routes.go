// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

// maximumRuleSetRequestBytes carries a rule set's largest rules, whose JSON
// escapes can grow them, and the rest of the request.
const maximumRuleSetRequestBytes = 2*store.MaximumRuleSetRulesBytes + (64 << 10)

func (handler *Handler) listSubscriptionRuleSets(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(w, request, "client")
	if !ok {
		return
	}
	client := store.SubscriptionFormat(query.Get("client"))
	if client != "" && client != store.SubscriptionFormatSingBox && client != store.SubscriptionFormatMihomo && client != store.SubscriptionFormatLoon {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "client must be sing-box, mihomo or loon.")
		return
	}
	sets, err := handler.services.Subscriptions.ListSubscriptionRuleSets(request.Context(), client)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_rule_set_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, sets)
}

func (handler *Handler) createSubscriptionRuleSet(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input subscription.CreateSubscriptionRuleSetRequest
	if !decodeStrictRequest(w, request, maximumRuleSetRequestBytes, &input) {
		return
	}
	set, err := handler.services.Subscriptions.CreateSubscriptionRuleSet(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_rule_set_create_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusCreated, set.UpdatedAt, set)
}

func (handler *Handler) setSubscriptionRuleSetOrder(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Client store.SubscriptionFormat `json:"client"`
		IDs    []string                 `json:"ids"`
	}
	if !decodeStrictRequest(w, request, 2<<20, &input) {
		return
	}
	if input.IDs == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	if err := handler.services.Subscriptions.SetSubscriptionRuleSetOrder(request.Context(), input.Client, input.IDs); err != nil {
		writeSubscriptionProblem(w, request, "subscription_rule_set_order_failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) getSubscriptionRuleSet(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	set, err := handler.services.Subscriptions.SubscriptionRuleSet(request.Context(), identifier)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_rule_set_read_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, set.UpdatedAt, set)
}

func (handler *Handler) updateSubscriptionRuleSet(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input subscription.UpdateSubscriptionRuleSetRequest
	if !decodeStrictRequest(w, request, maximumRuleSetRequestBytes, &input) {
		return
	}
	set, err := handler.services.Subscriptions.UpdateSubscriptionRuleSet(request.Context(), identifier, input, expected)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_rule_set_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, set.UpdatedAt, set)
}

func (handler *Handler) deleteSubscriptionRuleSet(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	if err := handler.services.Subscriptions.DeleteSubscriptionRuleSet(request.Context(), identifier, expected); err != nil {
		writeSubscriptionProblem(w, request, "subscription_rule_set_delete_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
