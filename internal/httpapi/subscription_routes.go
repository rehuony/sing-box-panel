// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strings"
)

func matchSubscriptionRoute(path string) (resource string, identifier string, operation string, matched bool) {
	const prefix = "/api/v1/subscription/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) == 1 && validSubscriptionResource(parts[0]) {
		return parts[0], "", "", true
	}
	if len(parts) == 2 && validSubscriptionResource(parts[0]) && parts[1] != "" {
		return parts[0], parts[1], "", true
	}
	if len(parts) == 3 && parts[1] != "" {
		switch {
		case parts[0] == "users" && parts[2] == "grants":
			return parts[0], parts[1], parts[2], true
		case parts[0] == "channels" && parts[2] == "preview":
			return parts[0], parts[1], parts[2], true
		case parts[0] == "sources" && parts[2] == "versions":
			return parts[0], parts[1], parts[2], true
		case parts[0] == "sources" && parts[2] == "refresh":
			return parts[0], parts[1], parts[2], true
		case parts[0] == "tokens" && (parts[2] == "rotate" || parts[2] == "revoke" ||
			parts[2] == "enable" || parts[2] == "disable"):
			return parts[0], parts[1], parts[2], true
		}
	}
	if len(parts) == 4 && parts[0] == "sources" && parts[1] != "" &&
		parts[2] == "versions" && parts[3] != "" {
		return parts[0], parts[1] + "/" + parts[3], "version", true
	}
	if len(parts) == 5 && parts[0] == "sources" && parts[1] != "" &&
		parts[2] == "versions" && parts[3] != "" && parts[4] == "restore" {
		return parts[0], parts[1] + "/" + parts[3], "restore", true
	}
	return "invalid", "", "invalid", true
}

func validSubscriptionResource(value string) bool {
	return value == "channels" || value == "sources" || value == "tokens" || value == "users" || value == "nodes"
}

func (handler *Handler) subscriptionManagementHandler(
	method string,
	resource string,
	identifier string,
	operation string,
) http.HandlerFunc {
	switch {
	case resource == "nodes" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.subscriptionNodeCatalog
	case resource == "users" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionUsers
	case resource == "users" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionUser
	case resource == "users" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionUser(w, request, identifier)
		}
	case resource == "users" && identifier != "" && operation == "" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.updateSubscriptionUser(w, request, identifier)
		}
	case resource == "users" && identifier != "" && operation == "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.deleteSubscriptionUser(w, request, identifier)
		}
	case resource == "users" && identifier != "" && operation == "grants" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionUserGrants(w, request, identifier)
		}
	case resource == "users" && identifier != "" && operation == "grants" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.replaceSubscriptionUserGrants(w, request, identifier)
		}
	case resource == "channels" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionChannels
	case resource == "channels" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionChannel
	case resource == "channels" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionChannel(w, request, identifier)
		}
	case resource == "channels" && identifier != "" && operation == "" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.updateSubscriptionChannel(w, request, identifier)
		}
	case resource == "channels" && identifier != "" && operation == "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.deleteSubscriptionChannel(w, request, identifier)
		}
	case resource == "channels" && identifier != "" && operation == "preview" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.previewSubscriptionChannel(w, request, identifier)
		}
	case resource == "sources" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionSources
	case resource == "sources" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionSource
	case resource == "sources" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "" && method == http.MethodPut:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.updateSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.deleteSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "versions" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.listSubscriptionSourceVersions(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "versions" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.createSubscriptionSourceVersion(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "refresh" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.refreshSubscriptionSource(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "version" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionSourceVersion(w, request, identifier)
		}
	case resource == "sources" && identifier != "" && operation == "restore" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.restoreSubscriptionSourceVersion(w, request, identifier)
		}
	case resource == "tokens" && identifier == "" && operation == "" && method == http.MethodGet:
		return handler.listSubscriptionTokens
	case resource == "tokens" && identifier == "" && operation == "" && method == http.MethodPost:
		return handler.createSubscriptionToken
	case resource == "tokens" && identifier != "" && operation == "" && method == http.MethodGet:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.getSubscriptionToken(w, request, identifier)
		}
	case resource == "tokens" && identifier != "" && operation == "rotate" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.rotateSubscriptionToken(w, request, identifier)
		}
	case resource == "tokens" && identifier != "" && operation == "revoke" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.revokeSubscriptionToken(w, request, identifier)
		}
	case resource == "tokens" && identifier != "" && operation == "enable" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.setSubscriptionTokenEnabled(w, request, identifier, true)
		}
	case resource == "tokens" && identifier != "" && operation == "disable" && method == http.MethodPost:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.setSubscriptionTokenEnabled(w, request, identifier, false)
		}
	case resource == "tokens" && identifier != "" && operation == "" && method == http.MethodDelete:
		return func(w http.ResponseWriter, request *http.Request) {
			handler.deleteSubscriptionToken(w, request, identifier)
		}
	default:
		return methodNotAllowed
	}
}

func (handler *Handler) subscriptionNodeCatalog(w http.ResponseWriter, request *http.Request) {
	if !handler.subscriptionReadRequest(w, request) {
		return
	}
	catalog, err := handler.commands.SubscriptionNodeCatalog(request.Context())
	if err != nil {
		writeSubscriptionProblem(w, request, "subscription_node_catalog_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}
