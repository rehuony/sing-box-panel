// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
)

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

func (handler *Handler) setSubscriptionTokenEnabled(
	w http.ResponseWriter,
	request *http.Request,
	identifier string,
	enabled bool,
) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	token, err := handler.commands.SetSubscriptionTokenEnabled(request.Context(), identifier, enabled)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_state_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, token)
}

func (handler *Handler) deleteSubscriptionToken(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	if err := handler.commands.DeleteSubscriptionToken(request.Context(), identifier); err != nil {
		writeSubscriptionProblem(w, request, "subscription_token_delete_failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
