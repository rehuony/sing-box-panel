// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func (handler *Handler) listSubscriptionUsers(w http.ResponseWriter, request *http.Request) {
	input, ok := handler.subscriptionListRequest(w, request)
	if !ok {
		return
	}
	users, err := handler.commands.ListSubscriptionUsers(request.Context(), input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (handler *Handler) createSubscriptionUser(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	user, err := handler.commands.CreateSubscriptionUser(request.Context(), application.CreateSubscriptionUserRequest{
		Name: input.Name, Description: input.Description, Enabled: *input.Enabled,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_create_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusCreated, user.UpdatedAt, user)
}

func (handler *Handler) getSubscriptionUser(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	user, err := handler.commands.SubscriptionUser(request.Context(), id)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_read_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, user.UpdatedAt, user)
}

func (handler *Handler) updateSubscriptionUser(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	user, err := handler.commands.UpdateSubscriptionUser(request.Context(), id, application.UpdateSubscriptionUserRequest{
		Name: input.Name, Description: input.Description, Enabled: *input.Enabled, ExpectedUpdatedAt: expected,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, user.UpdatedAt, user)
}

func (handler *Handler) deleteSubscriptionUser(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	if err := handler.commands.DeleteSubscriptionUser(request.Context(), id, expected); err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_delete_failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) getSubscriptionUserGrants(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	grants, err := handler.commands.SubscriptionUserGrants(request.Context(), id)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_grants_read_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, grants.User.UpdatedAt, grants)
}

func (handler *Handler) replaceSubscriptionUserGrants(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		Grants []string `json:"grants"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if input.Grants == nil {
		writeSubscriptionInvalid(w, request)
		return
	}
	grants, err := handler.commands.ReplaceSubscriptionUserGrants(request.Context(), id, input.Grants, expected)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_user_grants_update_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, grants.User.UpdatedAt, grants)
}
