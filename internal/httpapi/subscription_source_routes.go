// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription/source"
)

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
	source, err := handler.commands.CreateSubscriptionSource(request.Context(), application.CreateSubscriptionSourceRequest{
		Name: input.Name, SourceKind: input.SourceKind, Config: input.Config,
		Enabled: *input.Enabled,
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

func (handler *Handler) listSubscriptionSourceVersions(w http.ResponseWriter, request *http.Request, sourceID string) {
	input, ok := handler.subscriptionListRequest(w, request)
	if !ok {
		return
	}
	versions, err := handler.commands.ListSubscriptionSourceVersions(request.Context(), sourceID, input)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_versions_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (handler *Handler) refreshSubscriptionSource(w http.ResponseWriter, request *http.Request, sourceID string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	task, err := handler.commands.QueueSubscriptionSourceRefresh(request.Context(), sourceID)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_refresh_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) createSubscriptionSourceVersion(w http.ResponseWriter, request *http.Request, sourceID string) {
	if !handler.subscriptionMutationRequest(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	var input struct {
		Format  source.Format `json:"format"`
		RawBody []byte        `json:"raw_body"`
	}
	if !decodeStrictRequest(w, request, maximumSubscriptionRequestBytes, &input) {
		return
	}
	if len(input.RawBody) == 0 {
		writeSubscriptionInvalid(w, request)
		return
	}
	saved, err := handler.commands.CreateSubscriptionSourceVersion(request.Context(), sourceID, application.CreateSubscriptionSourceVersionRequest{
		Format: input.Format, RawBody: input.RawBody, ExpectedUpdatedAt: expected,
	})
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_version_create_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusCreated, saved.Source.UpdatedAt, saved)
}

func (handler *Handler) getSubscriptionSourceVersion(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	sourceID, versionID, ok := splitSubscriptionSourceVersionIdentifier(identifier)
	if !ok {
		writeSubscriptionInvalid(w, request)
		return
	}
	version, err := handler.commands.SubscriptionSourceVersion(request.Context(), sourceID, versionID)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_version_read_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

func (handler *Handler) restoreSubscriptionSourceVersion(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.subscriptionMutationRequest(w, request) || !requireEmptyCoreBody(w, request) {
		return
	}
	expected, ok := subscriptionIfMatch(w, request)
	if !ok {
		return
	}
	sourceID, versionID, ok := splitSubscriptionSourceVersionIdentifier(identifier)
	if !ok {
		writeSubscriptionInvalid(w, request)
		return
	}
	source, err := handler.commands.RestoreSubscriptionSourceVersion(request.Context(), sourceID, versionID, expected)
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_source_version_restore_failed", err)
		return
	}
	writeSubscriptionResource(w, http.StatusOK, source.UpdatedAt, source)
}

func splitSubscriptionSourceVersionIdentifier(identifier string) (string, string, bool) {
	parts := strings.Split(identifier, "/")
	valid := len(parts) == 2 && parts[0] != "" && parts[1] != ""
	if !valid {
		return "", "", false
	}
	return parts[0], parts[1], true
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
