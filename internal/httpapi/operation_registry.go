// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strings"
)

// managementOperation is one member of the closed public transport surface implemented by
// this binary. Paths use the OpenAPI templates relative to /api/v1, except for
// the public subscription download whose contract overrides the server base.
type managementOperation struct {
	Method      string
	Path        string
	OperationID string
}

var managementOperations = []managementOperation{
	{Method: http.MethodGet, Path: "/panel/settings", OperationID: "getPanelSettings"},
	{Method: http.MethodPut, Path: "/panel/settings", OperationID: "savePanelSettings"},
	{Method: http.MethodGet, Path: "/panel/backup", OperationID: "exportPanelBackup"},
	{Method: http.MethodPost, Path: "/panel/restore", OperationID: "restorePanelBackup"},
	{Method: http.MethodGet, Path: "/config/file", OperationID: "getConfigurationFile"},
	{Method: http.MethodPut, Path: "/config/file", OperationID: "saveConfigurationFile"},
	{Method: http.MethodPost, Path: "/config/inbound-defaults", OperationID: "newInboundDefaults"},
	{Method: http.MethodGet, Path: "/health", OperationID: "getHealth"},
	{Method: http.MethodGet, Path: "/auth/session", OperationID: "getSession"},
	{Method: http.MethodPost, Path: "/auth/session", OperationID: "createSession"},
	{Method: http.MethodDelete, Path: "/auth/session", OperationID: "deleteSession"},
	{Method: http.MethodGet, Path: "/system/status", OperationID: "getSystemStatus"},
	{Method: http.MethodGet, Path: "/dashboard/context", OperationID: "getDashboardContext"},
	{Method: http.MethodPost, Path: "/config/preview", OperationID: "previewConfiguration"},
	{Method: http.MethodPost, Path: "/config/compile", OperationID: "compileConfiguration"},
	{Method: http.MethodGet, Path: "/config/artifacts", OperationID: "listStartupArtifacts"},
	{Method: http.MethodPost, Path: "/config/apply", OperationID: "applyStartupArtifact"},
	{Method: http.MethodGet, Path: "/core/catalog/assets", OperationID: "listCoreCatalogAssets"},
	{Method: http.MethodPost, Path: "/core/catalog/refresh", OperationID: "refreshCoreCatalog"},
	{Method: http.MethodGet, Path: "/core/artifacts", OperationID: "listCoreArtifacts"},
	{Method: http.MethodGet, Path: "/core/artifacts/{artifactId}", OperationID: "getCoreArtifact"},
	{Method: http.MethodPost, Path: "/core/artifacts/{artifactId}/enable", OperationID: "enableCoreArtifact"},
	{Method: http.MethodPost, Path: "/core/artifacts/{artifactId}/disable", OperationID: "disableCoreArtifact"},
	{Method: http.MethodDelete, Path: "/core/artifacts/{artifactId}", OperationID: "deleteCoreArtifact"},
	{Method: http.MethodGet, Path: "/core/artifacts/{artifactId}/configuration-support", OperationID: "getCoreArtifactConfigurationSupport"},
	{Method: http.MethodGet, Path: "/core/artifacts/{artifactId}/configuration-schema", OperationID: "getCoreArtifactConfigurationSchema"},
	{Method: http.MethodPost, Path: "/core/install", OperationID: "installCoreArtifact"},
	{Method: http.MethodPost, Path: "/core/import", OperationID: "importCoreArtifact"},
	{Method: http.MethodGet, Path: "/core/status", OperationID: "getCoreRuntimeStatus"},
	{Method: http.MethodGet, Path: "/core/runtime/history", OperationID: "getCoreRuntimeHistory"},
	{Method: http.MethodPost, Path: "/core/check", OperationID: "checkStartupArtifact"},
	{Method: http.MethodPost, Path: "/core/activate", OperationID: "activateStartupArtifact"},
	{Method: http.MethodPost, Path: "/core/start", OperationID: "startCoreRuntime"},
	{Method: http.MethodPost, Path: "/core/stop", OperationID: "stopCoreRuntime"},
	{Method: http.MethodPost, Path: "/core/restart", OperationID: "restartCoreRuntime"},
	{Method: http.MethodPost, Path: "/core/rollback", OperationID: "rollbackCoreRuntime"},
	{Method: http.MethodGet, Path: "/subscription/nodes", OperationID: "listSubscriptionNodes"},
	{Method: http.MethodPost, Path: "/subscription/nodes", OperationID: "createSubscriptionNode"},
	{Method: http.MethodPost, Path: "/subscription/nodes/parse", OperationID: "parseSubscriptionNode"},
	{Method: http.MethodGet, Path: "/subscription/nodes/{nodeId}", OperationID: "getSubscriptionNode"},
	{Method: http.MethodPut, Path: "/subscription/nodes/{nodeId}", OperationID: "updateSubscriptionNode"},
	{Method: http.MethodDelete, Path: "/subscription/nodes/{nodeId}", OperationID: "deleteSubscriptionNode"},
	{Method: http.MethodPut, Path: "/subscription/nodes/{nodeId}/visibility", OperationID: "setSubscriptionNodeVisibility"},
	{Method: http.MethodGet, Path: "/subscription/users", OperationID: "listSubscriptionUsers"},
	{Method: http.MethodPost, Path: "/subscription/users", OperationID: "createSubscriptionUser"},
	{Method: http.MethodGet, Path: "/subscription/users/{userId}", OperationID: "getSubscriptionUser"},
	{Method: http.MethodPut, Path: "/subscription/users/{userId}", OperationID: "updateSubscriptionUser"},
	{Method: http.MethodDelete, Path: "/subscription/users/{userId}", OperationID: "deleteSubscriptionUser"},
	{Method: http.MethodGet, Path: "/subscription/users/{userId}/grants", OperationID: "getSubscriptionUserGrants"},
	{Method: http.MethodPut, Path: "/subscription/users/{userId}/grants", OperationID: "replaceSubscriptionUserGrants"},
	{Method: http.MethodGet, Path: "/subscription/channels", OperationID: "listSubscriptionChannels"},
	{Method: http.MethodPost, Path: "/subscription/channels", OperationID: "createSubscriptionChannel"},
	{Method: http.MethodGet, Path: "/subscription/channels/{channelId}", OperationID: "getSubscriptionChannel"},
	{Method: http.MethodPut, Path: "/subscription/channels/{channelId}", OperationID: "updateSubscriptionChannel"},
	{Method: http.MethodDelete, Path: "/subscription/channels/{channelId}", OperationID: "deleteSubscriptionChannel"},
	{Method: http.MethodPost, Path: "/subscription/channels/{channelId}/preview", OperationID: "previewSubscriptionChannel"},
	{Method: http.MethodGet, Path: "/subscription/sources", OperationID: "listSubscriptionSources"},
	{Method: http.MethodPost, Path: "/subscription/sources", OperationID: "createSubscriptionSource"},
	{Method: http.MethodGet, Path: "/subscription/sources/{sourceId}", OperationID: "getSubscriptionSource"},
	{Method: http.MethodPut, Path: "/subscription/sources/{sourceId}", OperationID: "updateSubscriptionSource"},
	{Method: http.MethodDelete, Path: "/subscription/sources/{sourceId}", OperationID: "deleteSubscriptionSource"},
	{Method: http.MethodPost, Path: "/subscription/sources/{sourceId}/refresh", OperationID: "refreshSubscriptionSource"},
	{Method: http.MethodGet, Path: "/subscription/sources/{sourceId}/versions", OperationID: "listSubscriptionSourceVersions"},
	{Method: http.MethodPost, Path: "/subscription/sources/{sourceId}/versions", OperationID: "createSubscriptionSourceVersion"},
	{Method: http.MethodGet, Path: "/subscription/sources/{sourceId}/versions/{versionId}", OperationID: "getSubscriptionSourceVersion"},
	{Method: http.MethodPost, Path: "/subscription/sources/{sourceId}/versions/{versionId}/restore", OperationID: "restoreSubscriptionSourceVersion"},
	{Method: http.MethodGet, Path: "/subscription/tokens", OperationID: "listSubscriptionTokens"},
	{Method: http.MethodPost, Path: "/subscription/tokens", OperationID: "createSubscriptionToken"},
	{Method: http.MethodGet, Path: "/subscription/tokens/{tokenId}/secret", OperationID: "getSubscriptionTokenSecret"},
	{Method: http.MethodGet, Path: "/subscription/tokens/{tokenId}", OperationID: "getSubscriptionToken"},
	{Method: http.MethodDelete, Path: "/subscription/tokens/{tokenId}", OperationID: "deleteSubscriptionToken"},
	{Method: http.MethodPost, Path: "/subscription/tokens/{tokenId}/rotate", OperationID: "rotateSubscriptionToken"},
	{Method: http.MethodPost, Path: "/subscription/tokens/{tokenId}/revoke", OperationID: "revokeSubscriptionToken"},
	{Method: http.MethodPost, Path: "/subscription/tokens/{tokenId}/enable", OperationID: "enableSubscriptionToken"},
	{Method: http.MethodPost, Path: "/subscription/tokens/{tokenId}/disable", OperationID: "disableSubscriptionToken"},
	{Method: http.MethodGet, Path: "/sub/{token}/{channelId}", OperationID: "downloadSubscription"},
	{Method: http.MethodGet, Path: "/logs", OperationID: "listLogs"},
	{Method: http.MethodDelete, Path: "/logs", OperationID: "clearLogs"},
	{Method: http.MethodGet, Path: "/logs/stream", OperationID: "streamLogs"},
	{Method: http.MethodGet, Path: "/logs/{logId}", OperationID: "getLog"},
	{Method: http.MethodDelete, Path: "/logs/{logId}", OperationID: "deleteLog"},
	{Method: http.MethodGet, Path: "/metrics/stream", OperationID: "streamMetrics"},
	{Method: http.MethodGet, Path: "/dashboard/stream", OperationID: "streamDashboard"},
	{Method: http.MethodGet, Path: "/core/logs/files", OperationID: "listCoreLogFiles"},
	{Method: http.MethodDelete, Path: "/core/logs/files", OperationID: "deleteCoreLogFile"},
	{Method: http.MethodDelete, Path: "/core/logs/content", OperationID: "clearCoreLog"},
	{Method: http.MethodGet, Path: "/core/logs/content", OperationID: "readCoreLog"},
	{Method: http.MethodGet, Path: "/core/logs/stream", OperationID: "streamCoreLog"},
	{Method: http.MethodGet, Path: "/logs/panel", OperationID: "listPanelLogs"},
	{Method: http.MethodGet, Path: "/metrics", OperationID: "getMetrics"},
	{Method: http.MethodGet, Path: "/metrics/history", OperationID: "getMetricsHistory"},
	{Method: http.MethodGet, Path: "/traffic/status", OperationID: "getTrafficStatus"},
	{Method: http.MethodGet, Path: "/traffic/periods", OperationID: "listTrafficPeriods"},
	{Method: http.MethodGet, Path: "/traffic/periods/{periodId}", OperationID: "getTrafficPeriod"},
}

func registeredManagementOperation(method, requestPath string) (managementOperation, bool, bool) {
	contractPath := strings.TrimPrefix(requestPath, "/api/v1")
	pathRegistered := false
	for _, operation := range managementOperations {
		if strings.HasPrefix(operation.Path, "/sub/") || !matchOperationTemplate(operation.Path, contractPath) {
			continue
		}
		pathRegistered = true
		if operation.Method == method {
			return operation, true, true
		}
	}
	return managementOperation{}, false, pathRegistered
}

func matchOperationTemplate(template, requestPath string) bool {
	templateParts := strings.Split(strings.Trim(template, "/"), "/")
	pathParts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(templateParts) != len(pathParts) {
		return false
	}
	for index, part := range templateParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			if pathParts[index] == "" {
				return false
			}
			continue
		}
		if part != pathParts[index] {
			return false
		}
	}
	return true
}
