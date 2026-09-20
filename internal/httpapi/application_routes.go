// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strings"
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
	} else if path == "/api/v1/config/file" {
		if request.Method == http.MethodGet {
			next = handler.configurationFile
		} else if request.Method == http.MethodPut {
			next = handler.saveConfigurationFile
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/inbound-defaults" {
		if request.Method == http.MethodPost {
			next = handler.newInboundDefaults
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/apply" {
		if request.Method == http.MethodPost {
			next = handler.queueCoreActivate
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/preview" {
		if request.Method == http.MethodPost {
			next = handler.previewConfiguration
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/compile" {
		if request.Method == http.MethodPost {
			next = handler.compileConfiguration
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/artifacts" {
		if request.Method == http.MethodGet {
			next = handler.listStartupArtifacts
		} else {
			next = methodNotAllowed
		}
	} else if resource, identifier, matched := matchObservabilityRoute(path); matched {
		next = handler.observabilityHandler(request.Method, resource, identifier)
	} else if resource, identifier, matched := matchCoreRoute(path); matched {
		switch {
		case resource == "catalog-assets" && request.Method == http.MethodGet:
			next = handler.listCatalogAssets
		case resource == "catalog-refresh" && request.Method == http.MethodPost:
			next = handler.queueCatalogRefresh
		case resource == "artifacts" && identifier == "" && request.Method == http.MethodGet:
			next = handler.listCoreArtifacts
		case resource == "artifacts" && identifier != "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.getCoreArtifact(w, request, identifier)
			}
		case resource == "artifacts" && identifier != "" && request.Method == http.MethodDelete:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.deleteCoreArtifact(w, request, identifier)
			}
		case resource == "artifact-enable" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.enableCoreArtifact(w, request, identifier)
			}
		case resource == "artifact-disable" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.disableCoreArtifact(w, request, identifier)
			}
		case resource == "artifact-configuration-support" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.coreConfigurationSupport(w, request, identifier)
			}
		case resource == "artifact-configuration-schema" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.coreConfigurationSchema(w, request, identifier)
			}
		case resource == "install" && request.Method == http.MethodPost:
			next = handler.queueCoreInstall
		case resource == "import" && request.Method == http.MethodPost:
			next = handler.queueCoreImport
		case resource == "status" && request.Method == http.MethodGet:
			next = handler.coreRuntimeStatus
		case resource == "runtime-history" && request.Method == http.MethodGet:
			next = handler.coreRuntimeHistory
		case resource == "check" && request.Method == http.MethodPost:
			next = handler.queueStartupCheck
		case resource == "activate" && request.Method == http.MethodPost:
			next = handler.queueCoreActivate
		case resource == "start" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "start")
			}
		case resource == "stop" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "stop")
			}
		case resource == "restart" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "restart")
			}
		case resource == "rollback" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "rollback")
			}
		default:
			next = methodNotAllowed
		}
	} else if resource, identifier, operation, matched := matchSubscriptionRoute(path); matched {
		next = handler.subscriptionManagementHandler(request.Method, resource, identifier, operation)
	} else if reference, operation, matched := matchRevisionRoute(path); matched {
		switch {
		case reference == "" && operation == "" && request.Method == http.MethodGet:
			next = handler.listRevisions
		case reference == "" && operation == "diff" && request.Method == http.MethodGet:
			next = handler.diffRevisions
		case reference != "" && operation == "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) { handler.getRevision(w, request, reference) }
		case reference != "" && operation == "restore" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) { handler.restoreRevision(w, request, reference) }
		default:
			next = methodNotAllowed
		}
	} else if taskID, operation, matched := matchTaskRoute(path); matched {
		switch {
		case taskID == "" && operation == "" && request.Method == http.MethodGet:
			next = handler.listTasks
		case taskID != "" && operation == "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) { handler.getTask(w, request, taskID) }
		case taskID != "" && operation == "retry" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) { handler.retryTask(w, request, taskID) }
		case taskID != "" && operation == "cancel" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) { handler.cancelTask(w, request, taskID) }
		default:
			next = methodNotAllowed
		}
	}
	if next == nil {
		return false
	}
	handler.authenticated(next)(w, request)
	return true
}

func matchCoreRoute(path string) (string, string, bool) {
	switch path {
	case "/api/v1/core/catalog/assets":
		return "catalog-assets", "", true
	case "/api/v1/core/catalog/refresh":
		return "catalog-refresh", "", true
	case "/api/v1/core/artifacts":
		return "artifacts", "", true
	case "/api/v1/core/install":
		return "install", "", true
	case "/api/v1/core/import":
		return "import", "", true
	case "/api/v1/core/status":
		return "status", "", true
	case "/api/v1/core/runtime/history":
		return "runtime-history", "", true
	case "/api/v1/core/check":
		return "check", "", true
	case "/api/v1/core/activate":
		return "activate", "", true
	case "/api/v1/core/start":
		return "start", "", true
	case "/api/v1/core/stop":
		return "stop", "", true
	case "/api/v1/core/restart":
		return "restart", "", true
	case "/api/v1/core/rollback":
		return "rollback", "", true
	}
	const artifactsPrefix = "/api/v1/core/artifacts/"
	if !strings.HasPrefix(path, artifactsPrefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(path, artifactsPrefix)
	parts := strings.Split(remainder, "/")
	if len(parts) == 1 && parts[0] != "" {
		return "artifacts", parts[0], true
	}
	if len(parts) == 2 && parts[0] != "" {
		switch parts[1] {
		case "configuration-support":
			return "artifact-configuration-support", parts[0], true
		case "configuration-schema":
			return "artifact-configuration-schema", parts[0], true
		case "enable":
			return "artifact-enable", parts[0], true
		case "disable":
			return "artifact-disable", parts[0], true
		}
	}
	if remainder == "" || strings.Contains(remainder, "/") {
		return "invalid", "", true
	}
	return "invalid", "", true
}

func matchRevisionRoute(path string) (string, string, bool) {
	const prefix = "/api/v1/config/revisions"
	if path == prefix {
		return "", "", true
	}
	if path == prefix+"/diff" {
		return "", "diff", true
	}
	if !strings.HasPrefix(path, prefix+"/") {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix+"/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "restore" {
		return parts[0], "restore", true
	}
	return "", "invalid", true
}

func matchTaskRoute(path string) (string, string, bool) {
	const prefix = "/api/v1/tasks"
	if path == prefix {
		return "", "", true
	}
	if !strings.HasPrefix(path, prefix+"/") {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix+"/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && (parts[1] == "cancel" || parts[1] == "retry") {
		return parts[0], parts[1], true
	}
	return "", "invalid", true
}

func methodNotAllowed(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "The resource does not support this HTTP method.")
}

func (handler *Handler) requireCommands(w http.ResponseWriter, request *http.Request) bool {
	if handler.commands != nil {
		return true
	}
	writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Configuration services are not ready.")
	return false
}
