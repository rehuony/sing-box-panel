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
	{Method: http.MethodGet, Path: "/health", OperationID: "getHealth"},
	{Method: http.MethodGet, Path: "/auth/session", OperationID: "getSession"},
	{Method: http.MethodPost, Path: "/auth/session", OperationID: "createSession"},
	{Method: http.MethodDelete, Path: "/auth/session", OperationID: "deleteSession"},
	{Method: http.MethodGet, Path: "/system/status", OperationID: "getSystemStatus"},
	{Method: http.MethodGet, Path: "/dashboard/context", OperationID: "getDashboardContext"},
	{Method: http.MethodGet, Path: "/config/canonical", OperationID: "getCanonicalConfiguration"},
	{Method: http.MethodPut, Path: "/config/canonical", OperationID: "replaceCanonicalConfiguration"},
	{Method: http.MethodPatch, Path: "/config/canonical", OperationID: "patchCanonicalConfiguration"},
	{Method: http.MethodPost, Path: "/config/preview", OperationID: "previewConfiguration"},
	{Method: http.MethodPost, Path: "/config/compile", OperationID: "compileConfiguration"},
	{Method: http.MethodGet, Path: "/config/artifacts", OperationID: "listStartupArtifacts"},
	{Method: http.MethodPost, Path: "/config/apply", OperationID: "applyStartupArtifact"},
	{Method: http.MethodGet, Path: "/config/revisions", OperationID: "listCanonicalRevisions"},
	{Method: http.MethodGet, Path: "/config/revisions/diff", OperationID: "diffCanonicalRevisions"},
	{Method: http.MethodGet, Path: "/config/revisions/{revisionId}", OperationID: "getCanonicalRevision"},
	{Method: http.MethodPost, Path: "/config/revisions/{revisionId}/restore", OperationID: "restoreCanonicalRevision"},
	{Method: http.MethodGet, Path: "/core/catalog/assets", OperationID: "listCoreCatalogAssets"},
	{Method: http.MethodPost, Path: "/core/catalog/refresh", OperationID: "refreshCoreCatalog"},
	{Method: http.MethodGet, Path: "/core/artifacts", OperationID: "listCoreArtifacts"},
	{Method: http.MethodGet, Path: "/core/artifacts/{artifactId}", OperationID: "getCoreArtifact"},
	{Method: http.MethodDelete, Path: "/core/artifacts/{artifactId}", OperationID: "deleteCoreArtifact"},
	{Method: http.MethodGet, Path: "/core/artifacts/{artifactId}/configuration-support", OperationID: "getCoreArtifactConfigurationSupport"},
	{Method: http.MethodPost, Path: "/core/artifacts/{artifactId}/quarantine", OperationID: "quarantineCoreArtifact"},
	{Method: http.MethodPost, Path: "/core/artifacts/{artifactId}/revoke", OperationID: "revokeCoreArtifact"},
	{Method: http.MethodPost, Path: "/core/install", OperationID: "installCoreArtifact"},
	{Method: http.MethodPost, Path: "/core/import", OperationID: "importCoreArtifact"},
	{Method: http.MethodGet, Path: "/core/status", OperationID: "getCoreRuntimeStatus"},
	{Method: http.MethodPost, Path: "/core/check", OperationID: "checkStartupArtifact"},
	{Method: http.MethodPost, Path: "/core/activate", OperationID: "activateStartupArtifact"},
	{Method: http.MethodPost, Path: "/core/start", OperationID: "startCoreRuntime"},
	{Method: http.MethodPost, Path: "/core/stop", OperationID: "stopCoreRuntime"},
	{Method: http.MethodPost, Path: "/core/restart", OperationID: "restartCoreRuntime"},
	{Method: http.MethodPost, Path: "/core/rollback", OperationID: "rollbackCoreRuntime"},
	{Method: http.MethodGet, Path: "/tasks", OperationID: "listTasks"},
	{Method: http.MethodGet, Path: "/tasks/{taskId}", OperationID: "getTask"},
	{Method: http.MethodPost, Path: "/tasks/{taskId}/cancel", OperationID: "cancelTask"},
	{Method: http.MethodGet, Path: "/subscription/nodes", OperationID: "listSubscriptionNodes"},
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
	{Method: http.MethodGet, Path: "/metrics", OperationID: "getMetrics"},
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
