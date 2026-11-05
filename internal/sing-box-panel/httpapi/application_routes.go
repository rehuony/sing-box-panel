// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
)

const (
	maximumCoreInstallRequestBytes = 4 << 10
	maximumCoreImportRequestBytes  = 32 << 10
	maximumCoreUploadBytes         = 128 << 20
	maximumCoreEmptyRequestBytes   = 1 << 10
	maximumCoreQueryBytes          = 8 << 10
	maximumRuntimeRequestBytes     = 4 << 10
)

func (handler *Handler) handleApplicationRoute(w http.ResponseWriter, request *http.Request, path string) bool {
	var next http.HandlerFunc
	if path == "/api/v1/panel/settings" {
		if request.Method == http.MethodGet {
			next = handler.getPanelSettings
		} else if request.Method == http.MethodPut {
			next = handler.savePanelSettings
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/panel/backup" {
		if request.Method == http.MethodGet {
			next = handler.exportPanelBackup
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/panel/restore" {
		if request.Method == http.MethodPost {
			next = handler.restorePanelBackup
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/core/catalog/assets" {
		if request.Method == http.MethodGet {
			next = handler.listCatalogAssets
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/core/catalog/refresh" {
		if request.Method == http.MethodPost {
			next = handler.refreshCatalog
		} else {
			next = methodNotAllowed
		}
	} else if resource, identifier, matched := matchObservabilityRoute(path); matched {
		next = handler.observabilityHandler(request.Method, resource, identifier)
	} else if route := handler.nodeRoute(request.Method, path); route != nil {
		next = route
	} else if route := handler.fleetRoute(request.Method, path); route != nil {
		next = route
	} else if resource, identifier, operation, matched := matchSubscriptionRoute(path); matched {
		next = handler.subscriptionManagementHandler(request.Method, resource, identifier, operation)
	}

	if next == nil {
		return false
	}
	handler.authenticated(next)(w, request)
	return true
}

func methodNotAllowed(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "The resource does not support this HTTP method.")
}

func (handler *Handler) requireCommands(w http.ResponseWriter, request *http.Request) bool {
	if handler.services.Fleet != nil {
		return true
	}
	writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Configuration services are not ready.")
	return false
}
