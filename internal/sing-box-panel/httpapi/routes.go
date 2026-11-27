// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// routeAccess is what a route requires before its handler runs.
type routeAccess int

const (
	// accessPublic routes run without a session.
	accessPublic routeAccess = iota
	// accessSession routes require an administrator session.
	accessSession
	// accessNode routes require a session and an existing {nodeId} node.
	accessNode
	// accessDownload is the public subscription download. ServeHTTP serves it
	// outside /api/v1 so every malformed or denied request looks the same.
	accessDownload
)

type routeHandler func(*Handler, http.ResponseWriter, *http.Request)

// route is one operation of the panel's management contract. Paths are the
// OpenAPI templates relative to /api/v1, except the public subscription
// download whose contract overrides the server base.
type route struct {
	Method      string
	Path        string
	OperationID string
	access      routeAccess
	handle      routeHandler
}

// routes is the complete management surface: the dispatcher serves exactly
// these operations, and a test keeps them equal to the OpenAPI contract.
var routes = []route{
	{http.MethodGet, "/health", "getHealth", accessPublic, (*Handler).health},
	{http.MethodGet, "/auth/session", "getSession", accessSession, (*Handler).currentSession},
	{http.MethodPost, "/auth/session", "createSession", accessPublic, (*Handler).login},
	{http.MethodDelete, "/auth/session", "deleteSession", accessSession, (*Handler).logout},
	{http.MethodGet, "/system/status", "getSystemStatus", accessSession, (*Handler).systemStatus},
	{http.MethodPost, "/system/restart", "restartSystem", accessSession, (*Handler).restartSystem},
	{http.MethodGet, "/system/update", "getSystemUpdate", accessSession, (*Handler).systemUpdate},
	{http.MethodPost, "/system/update", "startSystemUpdate", accessSession, (*Handler).startSystemUpdate},
	{http.MethodPost, "/system/update/check", "checkSystemUpdate", accessSession, (*Handler).checkSystemUpdate},

	{http.MethodGet, "/panel/settings", "getPanelSettings", accessSession, (*Handler).getPanelSettings},
	{http.MethodPut, "/panel/settings", "savePanelSettings", accessSession, (*Handler).savePanelSettings},
	{http.MethodGet, "/panel/backup", "exportPanelBackup", accessSession, (*Handler).exportPanelBackup},
	{http.MethodPost, "/panel/restore", "restorePanelBackup", accessSession, (*Handler).restorePanelBackup},

	{http.MethodGet, "/core/catalog/assets", "listCoreCatalogAssets", accessSession, (*Handler).listCatalogAssets},
	{http.MethodPost, "/core/catalog/refresh", "refreshCoreCatalog", accessSession, (*Handler).refreshCatalog},

	{http.MethodGet, "/fleet/status", "getFleetStatus", accessSession, (*Handler).fleetStatus},
	{http.MethodGet, "/fleet/stream", "streamFleetStatus", accessSession, (*Handler).streamFleetStatus},
	{http.MethodPost, "/fleet/commands", "runFleetCommand", accessSession, (*Handler).fleetCommands},
	{http.MethodGet, "/fleet/runtime/history", "listFleetRuntimeHistory", accessSession, (*Handler).fleetRuntimeHistory},
	{http.MethodGet, "/nodes", "listNodes", accessSession, (*Handler).listNodes},
	{http.MethodPost, "/nodes", "createNode", accessSession, (*Handler).createNode},
	{http.MethodGet, "/nodes/{nodeId}", "getNode", accessSession, withPathValue("nodeId", (*Handler).getNode)},
	{http.MethodPut, "/nodes/{nodeId}", "updateNode", accessSession, withPathValue("nodeId", (*Handler).updateNode)},
	{http.MethodDelete, "/nodes/{nodeId}", "deleteNode", accessSession, withPathValue("nodeId", (*Handler).deleteNode)},
	{http.MethodPost, "/nodes/{nodeId}/enrollment", "createNodeEnrollment", accessSession, withPathValue("nodeId", (*Handler).createNodeEnrollment)},
	{http.MethodDelete, "/nodes/{nodeId}/enrollment", "cancelNodeEnrollment", accessSession, withPathValue("nodeId", (*Handler).cancelNodeEnrollment)},
	{http.MethodGet, "/nodes/{nodeId}/settings", "getNodeSettings", accessNode, (*Handler).nodeSettings},
	{http.MethodPut, "/nodes/{nodeId}/settings", "saveNodeSettings", accessNode, (*Handler).saveNodeSettings},
	{http.MethodPost, "/nodes/{nodeId}/update", "updateNodeRelease", accessNode, (*Handler).updateNodeRelease},

	{http.MethodGet, "/nodes/{nodeId}/filesystem/entries", "listFilesystemEntries", accessNode, (*Handler).listFilesystemEntries},
	{http.MethodGet, "/nodes/{nodeId}/filesystem/resolve", "resolveFilesystemPath", accessNode, (*Handler).resolveFilesystemPath},

	{http.MethodGet, "/nodes/{nodeId}/config/file", "getConfigurationFile", accessNode, (*Handler).configurationFile},
	{http.MethodPut, "/nodes/{nodeId}/config/file", "saveConfigurationFile", accessNode, (*Handler).saveConfigurationFile},
	{http.MethodPost, "/nodes/{nodeId}/config/preview", "previewConfiguration", accessNode, (*Handler).previewConfiguration},
	{http.MethodPost, "/nodes/{nodeId}/config/compile", "compileConfiguration", accessNode, (*Handler).compileConfiguration},

	{http.MethodGet, "/nodes/{nodeId}/core/artifacts", "listCoreArtifacts", accessNode, (*Handler).listCoreArtifacts},
	{http.MethodGet, "/nodes/{nodeId}/core/artifacts/{artifactId}", "getCoreArtifact", accessNode, withCoreArtifactID((*Handler).getCoreArtifact)},
	{http.MethodDelete, "/nodes/{nodeId}/core/artifacts/{artifactId}", "deleteCoreArtifact", accessNode, withCoreArtifactID((*Handler).deleteCoreArtifact)},
	{http.MethodPost, "/nodes/{nodeId}/core/artifacts/{artifactId}/enable", "enableCoreArtifact", accessNode, withCoreArtifactID((*Handler).enableCoreArtifact)},
	{http.MethodPost, "/nodes/{nodeId}/core/artifacts/{artifactId}/disable", "disableCoreArtifact", accessNode, withCoreArtifactID((*Handler).disableCoreArtifact)},
	{http.MethodGet, "/nodes/{nodeId}/core/artifacts/{artifactId}/configuration-support", "getCoreArtifactConfigurationSupport", accessNode, withCoreArtifactID((*Handler).coreConfigurationSupport)},
	{http.MethodGet, "/nodes/{nodeId}/core/artifacts/{artifactId}/configuration-schema", "getCoreArtifactConfigurationSchema", accessNode, withCoreArtifactID((*Handler).coreConfigurationSchema)},
	{http.MethodPost, "/nodes/{nodeId}/core/install", "installCoreArtifact", accessNode, (*Handler).installCore},
	{http.MethodPost, "/nodes/{nodeId}/core/import", "importCoreArtifact", accessNode, (*Handler).importCore},
	{http.MethodGet, "/nodes/{nodeId}/core/status", "getCoreRuntimeStatus", accessNode, (*Handler).coreRuntimeStatus},
	{http.MethodGet, "/nodes/{nodeId}/core/runtime/history", "getCoreRuntimeHistory", accessNode, (*Handler).coreRuntimeHistory},
	{http.MethodPost, "/nodes/{nodeId}/core/check", "checkStartupArtifact", accessNode, (*Handler).checkStartup},
	{http.MethodPost, "/nodes/{nodeId}/core/start", "startCoreRuntime", accessNode, runtimeLifecycle("start")},
	{http.MethodPost, "/nodes/{nodeId}/core/stop", "stopCoreRuntime", accessNode, runtimeLifecycle("stop")},
	{http.MethodPost, "/nodes/{nodeId}/core/restart", "restartCoreRuntime", accessNode, runtimeLifecycle("restart")},
	{http.MethodPost, "/nodes/{nodeId}/core/rollback", "rollbackCoreRuntime", accessNode, runtimeLifecycle("rollback")},
	{http.MethodGet, "/nodes/{nodeId}/core/logs/files", "listCoreLogFiles", accessNode, (*Handler).listCoreLogFiles},
	{http.MethodDelete, "/nodes/{nodeId}/core/logs/files", "deleteCoreLogFile", accessNode, (*Handler).deleteCoreLogFile},
	{http.MethodGet, "/nodes/{nodeId}/core/logs/content", "readCoreLog", accessNode, readCoreLog(false)},
	{http.MethodDelete, "/nodes/{nodeId}/core/logs/content", "clearCoreLog", accessNode, (*Handler).clearCoreLog},
	{http.MethodGet, "/nodes/{nodeId}/core/logs/stream", "streamCoreLog", accessNode, readCoreLog(true)},

	{http.MethodGet, "/nodes/{nodeId}/metrics", "getMetrics", accessNode, (*Handler).currentMetrics},
	{http.MethodGet, "/nodes/{nodeId}/metrics/stream", "streamMetrics", accessNode, (*Handler).streamMetrics},
	{http.MethodGet, "/nodes/{nodeId}/metrics/history", "getMetricsHistory", accessNode, (*Handler).metricsHistory},
	{http.MethodGet, "/nodes/{nodeId}/metrics/recent", "getRecentMetrics", accessNode, (*Handler).recentMetrics},
	{http.MethodGet, "/nodes/{nodeId}/dashboard/stream", "streamDashboard", accessNode, (*Handler).streamDashboard},
	{http.MethodGet, "/nodes/{nodeId}/traffic/periods", "listTrafficPeriods", accessNode, (*Handler).listTrafficPeriods},
	{http.MethodGet, "/nodes/{nodeId}/traffic/periods/{periodId}", "getTrafficPeriod", accessNode, withPathValue("periodId", (*Handler).getTrafficPeriod)},
	{http.MethodGet, "/traffic/status", "getTrafficStatus", accessSession, (*Handler).trafficStatus},

	{http.MethodGet, "/logs", "listLogs", accessSession, (*Handler).listDurableLogs},
	{http.MethodDelete, "/logs", "clearLogs", accessSession, (*Handler).clearDurableLogs},
	{http.MethodGet, "/logs/stream", "streamLogs", accessSession, (*Handler).streamDurableLogs},
	{http.MethodGet, "/logs/panel", "listPanelLogs", accessSession, (*Handler).listPanelLogs},
	{http.MethodGet, "/logs/usage", "getLogUsage", accessSession, (*Handler).logUsage},
	{http.MethodGet, "/logs/{logId}", "getLog", accessSession, withPathValue("logId", (*Handler).getDurableLog)},
	{http.MethodDelete, "/logs/{logId}", "deleteLog", accessSession, withPathValue("logId", (*Handler).deleteDurableLog)},

	{http.MethodGet, "/subscription/nodes", "listSubscriptionNodes", accessSession, (*Handler).listSubscriptionNodes},
	{http.MethodPut, "/subscription/node-order", "setSubscriptionNodeOrder", accessSession, (*Handler).setSubscriptionNodeOrder},
	{http.MethodGet, "/subscription/nodes/{nodeId}", "getSubscriptionNode", accessSession, withPathValue("nodeId", (*Handler).getSubscriptionNode)},
	{http.MethodPut, "/subscription/nodes/{nodeId}", "updateSubscriptionNode", accessSession, withPathValue("nodeId", (*Handler).updateSubscriptionNode)},

	{http.MethodGet, "/subscription/users", "listSubscriptionUsers", accessSession, (*Handler).listSubscriptionUsers},
	{http.MethodPost, "/subscription/users", "createSubscriptionUser", accessSession, (*Handler).createSubscriptionUser},
	{http.MethodGet, "/subscription/users/{userId}", "getSubscriptionUser", accessSession, withPathValue("userId", (*Handler).getSubscriptionUser)},
	{http.MethodPut, "/subscription/users/{userId}", "updateSubscriptionUser", accessSession, withPathValue("userId", (*Handler).updateSubscriptionUser)},
	{http.MethodDelete, "/subscription/users/{userId}", "deleteSubscriptionUser", accessSession, withPathValue("userId", (*Handler).deleteSubscriptionUser)},
	{http.MethodGet, "/subscription/users/{userId}/grants", "getSubscriptionUserGrants", accessSession, withPathValue("userId", (*Handler).getSubscriptionUserGrants)},
	{http.MethodPut, "/subscription/users/{userId}/grants", "replaceSubscriptionUserGrants", accessSession, withPathValue("userId", (*Handler).replaceSubscriptionUserGrants)},

	{http.MethodGet, "/subscription/channels", "listSubscriptionChannels", accessSession, (*Handler).listSubscriptionChannels},
	{http.MethodPost, "/subscription/channels", "createSubscriptionChannel", accessSession, (*Handler).createSubscriptionChannel},
	{http.MethodGet, "/subscription/channels/{channelId}", "getSubscriptionChannel", accessSession, withPathValue("channelId", (*Handler).getSubscriptionChannel)},
	{http.MethodPut, "/subscription/channels/{channelId}", "updateSubscriptionChannel", accessSession, withPathValue("channelId", (*Handler).updateSubscriptionChannel)},
	{http.MethodDelete, "/subscription/channels/{channelId}", "deleteSubscriptionChannel", accessSession, withPathValue("channelId", (*Handler).deleteSubscriptionChannel)},
	{http.MethodPost, "/subscription/channels/{channelId}/preview", "previewSubscriptionChannel", accessSession, withPathValue("channelId", (*Handler).previewSubscriptionChannel)},

	{http.MethodGet, "/subscription/rule-sets", "listSubscriptionRuleSets", accessSession, (*Handler).listSubscriptionRuleSets},
	{http.MethodPost, "/subscription/rule-sets", "createSubscriptionRuleSet", accessSession, (*Handler).createSubscriptionRuleSet},
	{http.MethodPut, "/subscription/rule-set-order", "setSubscriptionRuleSetOrder", accessSession, (*Handler).setSubscriptionRuleSetOrder},
	{http.MethodGet, "/subscription/rule-sets/{ruleSetId}", "getSubscriptionRuleSet", accessSession, withPathValue("ruleSetId", (*Handler).getSubscriptionRuleSet)},
	{http.MethodPut, "/subscription/rule-sets/{ruleSetId}", "updateSubscriptionRuleSet", accessSession, withPathValue("ruleSetId", (*Handler).updateSubscriptionRuleSet)},
	{http.MethodDelete, "/subscription/rule-sets/{ruleSetId}", "deleteSubscriptionRuleSet", accessSession, withPathValue("ruleSetId", (*Handler).deleteSubscriptionRuleSet)},

	{http.MethodGet, "/subscription/sources", "listSubscriptionSources", accessSession, (*Handler).listSubscriptionSources},
	{http.MethodPost, "/subscription/sources", "createSubscriptionSource", accessSession, (*Handler).createSubscriptionSource},
	{http.MethodPost, "/subscription/sources/preview", "previewSubscriptionSource", accessSession, (*Handler).previewSubscriptionSource},
	{http.MethodGet, "/subscription/sources/{sourceId}", "getSubscriptionSource", accessSession, withPathValue("sourceId", (*Handler).getSubscriptionSource)},
	{http.MethodPut, "/subscription/sources/{sourceId}", "updateSubscriptionSource", accessSession, withPathValue("sourceId", (*Handler).updateSubscriptionSource)},
	{http.MethodDelete, "/subscription/sources/{sourceId}", "deleteSubscriptionSource", accessSession, withPathValue("sourceId", (*Handler).deleteSubscriptionSource)},
	{http.MethodPost, "/subscription/sources/{sourceId}/refresh", "refreshSubscriptionSource", accessSession, withPathValue("sourceId", (*Handler).refreshSubscriptionSource)},

	{http.MethodGet, "/subscription/tokens", "listSubscriptionTokens", accessSession, (*Handler).listSubscriptionTokens},
	{http.MethodPost, "/subscription/tokens", "createSubscriptionToken", accessSession, (*Handler).createSubscriptionToken},
	{http.MethodGet, "/subscription/tokens/{tokenId}", "getSubscriptionToken", accessSession, withPathValue("tokenId", (*Handler).getSubscriptionToken)},
	{http.MethodDelete, "/subscription/tokens/{tokenId}", "deleteSubscriptionToken", accessSession, withPathValue("tokenId", (*Handler).deleteSubscriptionToken)},
	{http.MethodGet, "/subscription/tokens/{tokenId}/secret", "getSubscriptionTokenSecret", accessSession, withPathValue("tokenId", (*Handler).getSubscriptionTokenSecret)},
	{http.MethodPost, "/subscription/tokens/{tokenId}/rotate", "rotateSubscriptionToken", accessSession, withPathValue("tokenId", (*Handler).rotateSubscriptionToken)},
	{http.MethodPost, "/subscription/tokens/{tokenId}/revoke", "revokeSubscriptionToken", accessSession, withPathValue("tokenId", (*Handler).revokeSubscriptionToken)},
	{http.MethodPost, "/subscription/tokens/{tokenId}/enable", "enableSubscriptionToken", accessSession, subscriptionTokenEnabled(true)},
	{http.MethodPost, "/subscription/tokens/{tokenId}/disable", "disableSubscriptionToken", accessSession, subscriptionTokenEnabled(false)},

	{http.MethodGet, "/sub/{token}/{channelId}", "downloadSubscription", accessDownload, nil},
}

// serveOperation serves a /api/v1 request, given as the path below /api/v1.
// A known path with another method answers 405 before authentication.
func (handler *Handler) serveOperation(w http.ResponseWriter, request *http.Request, contractPath string) {
	matched, found, pathFound := matchRoute(request, contractPath)
	if !found {
		if pathFound {
			methodNotAllowed(w, request)
			return
		}
		writeProblem(w, request, http.StatusNotFound, "operation_not_found", "Operation not found", "The API operation does not exist.")
		return
	}
	next := func(w http.ResponseWriter, request *http.Request) { matched.handle(handler, w, request) }
	switch matched.access {
	case accessSession:
		next = handler.authenticated(next)
	case accessNode:
		next = handler.authenticated(handler.requireNode(next))
	}
	next(w, request)
}

// matchRoute selects the route for a path below /api/v1 and sets its path
// values on request. The path that matches with the most literal segments
// wins whatever the method, so /logs/panel is never read as a log ID: a
// method that path lacks is not allowed rather than routed elsewhere.
func matchRoute(request *http.Request, contractPath string) (route, bool, bool) {
	segments := strings.Split(strings.TrimPrefix(contractPath, "/"), "/")
	literalsByRoute := make(map[int]int)
	bestLiterals := -1
	for index, candidate := range routes {
		if candidate.access == accessDownload {
			continue
		}
		literals, ok := matchRouteTemplate(candidate.Path, segments)
		if !ok {
			continue
		}
		literalsByRoute[index] = literals
		bestLiterals = max(bestLiterals, literals)
	}
	if bestLiterals < 0 {
		return route{}, false, false
	}
	best := -1
	for index := range routes {
		if literals, ok := literalsByRoute[index]; ok && literals == bestLiterals && routes[index].Method == request.Method {
			best = index
			break
		}
	}
	if best < 0 {
		return route{}, false, true
	}
	matched := routes[best]
	for index, part := range strings.Split(strings.TrimPrefix(matched.Path, "/"), "/") {
		if name, ok := templateVariable(part); ok {
			request.SetPathValue(name, segments[index])
		}
	}
	return matched, true, true
}

// matchRouteTemplate reports whether segments fill template, and how many
// of its segments are literal.
func matchRouteTemplate(template string, segments []string) (int, bool) {
	parts := strings.Split(strings.TrimPrefix(template, "/"), "/")
	if len(parts) != len(segments) {
		return 0, false
	}
	literals := 0
	for index, part := range parts {
		if _, ok := templateVariable(part); ok {
			if segments[index] == "" {
				return 0, false
			}
			continue
		}
		if part != segments[index] {
			return 0, false
		}
		literals++
	}
	return literals, true
}

func templateVariable(part string) (string, bool) {
	name, ok := strings.CutPrefix(part, "{")
	if !ok {
		return "", false
	}
	return strings.CutSuffix(name, "}")
}

// requireNode runs next only for an existing {nodeId} node.
func (handler *Handler) requireNode(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if _, err := handler.services.Fleet.Node(request.Context(), requestNodeID(request)); err != nil {
			writeFleetProblem(w, request, err)
			return
		}
		next(w, request)
	}
}

// withPathValue passes the path value name to a handler that takes it.
func withPathValue(name string, next func(*Handler, http.ResponseWriter, *http.Request, string)) routeHandler {
	return func(handler *Handler, w http.ResponseWriter, request *http.Request) {
		next(handler, w, request, request.PathValue(name))
	}
}

// withCoreArtifactID rejects a malformed {artifactId} before the node is asked.
func withCoreArtifactID(next func(*Handler, http.ResponseWriter, *http.Request, string)) routeHandler {
	return func(handler *Handler, w http.ResponseWriter, request *http.Request) {
		identifier := request.PathValue("artifactId")
		if !store.ValidIdentifier(identifier) {
			writeProblem(w, request, http.StatusBadRequest, "core_artifact_id_invalid", "Core artifact ID invalid", "The core artifact ID is invalid.")
			return
		}
		next(handler, w, request, identifier)
	}
}

func runtimeLifecycle(operation string) routeHandler {
	return func(handler *Handler, w http.ResponseWriter, request *http.Request) {
		handler.runtimeLifecycle(w, request, operation)
	}
}

func readCoreLog(follow bool) routeHandler {
	return func(handler *Handler, w http.ResponseWriter, request *http.Request) {
		handler.readCoreLog(w, request, follow)
	}
}

func subscriptionTokenEnabled(enabled bool) routeHandler {
	return func(handler *Handler, w http.ResponseWriter, request *http.Request) {
		handler.setSubscriptionTokenEnabled(w, request, request.PathValue("tokenId"), enabled)
	}
}

func methodNotAllowed(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "The resource does not support this HTTP method.")
}
