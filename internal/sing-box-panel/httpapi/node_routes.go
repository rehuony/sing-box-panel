// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// nodeRoute selects the handler of a resource of one node, a path under
// /api/v1/nodes/{nodeId}/ in the config, core or filesystem groups, or nil
// when the path is not one. The handler reads the node from the nodeId path
// value after checking that the node exists.
func (handler *Handler) nodeRoute(method, path string) http.HandlerFunc {
	rest, ok := strings.CutPrefix(path, "/api/v1/nodes/")
	if !ok {
		return nil
	}
	nodeID, resource, ok := strings.Cut(rest, "/")
	if !ok || nodeID == "" {
		return nil
	}
	next := handler.nodeResource(method, "/"+resource)
	if next == nil {
		return nil
	}
	return func(w http.ResponseWriter, request *http.Request) {
		if handler.services.Fleet == nil {
			writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Node services are not ready.")
			return
		}
		if _, err := handler.services.Fleet.Node(request.Context(), nodeID); err != nil {
			writeFleetProblem(w, request, err)
			return
		}
		request.SetPathValue("nodeId", nodeID)
		next(w, request)
	}
}

func (handler *Handler) nodeResource(method, resource string) http.HandlerFunc {
	byMethod := map[string]http.HandlerFunc{}
	switch resource {
	case "/filesystem/entries":
		byMethod[http.MethodGet] = handler.listFilesystemEntries
	case "/filesystem/resolve":
		byMethod[http.MethodGet] = handler.resolveFilesystemPath
	case "/config/file":
		byMethod[http.MethodGet] = handler.configurationFile
		byMethod[http.MethodPut] = handler.saveConfigurationFile
	case "/config/apply":
		byMethod[http.MethodPost] = handler.activateCore
	case "/config/preview":
		byMethod[http.MethodPost] = handler.previewConfiguration
	case "/config/compile":
		byMethod[http.MethodPost] = handler.compileConfiguration
	case "/config/artifacts":
		byMethod[http.MethodGet] = handler.listStartupArtifacts
	case "/core/artifacts":
		byMethod[http.MethodGet] = handler.listCoreArtifacts
	case "/core/install":
		byMethod[http.MethodPost] = handler.installCore
	case "/core/import":
		byMethod[http.MethodPost] = handler.importCore
	case "/core/status":
		byMethod[http.MethodGet] = handler.coreRuntimeStatus
	case "/core/check":
		byMethod[http.MethodPost] = handler.checkStartup
	case "/core/activate":
		byMethod[http.MethodPost] = handler.activateCore
	case "/core/start", "/core/stop", "/core/restart", "/core/rollback":
		operation := strings.TrimPrefix(resource, "/core/")
		byMethod[http.MethodPost] = func(w http.ResponseWriter, request *http.Request) { handler.runtimeLifecycle(w, request, operation) }
	case "/core/logs/files":
		byMethod[http.MethodGet] = handler.listCoreLogFiles
		byMethod[http.MethodDelete] = handler.deleteCoreLogFile
	case "/core/logs/content":
		byMethod[http.MethodGet] = func(w http.ResponseWriter, request *http.Request) { handler.readCoreLog(w, request, false) }
		byMethod[http.MethodDelete] = handler.clearCoreLog
	case "/core/logs/stream":
		byMethod[http.MethodGet] = func(w http.ResponseWriter, request *http.Request) { handler.readCoreLog(w, request, true) }
	default:
		identifier, action, ok := matchCoreArtifactRoute(resource)
		if !ok {
			return nil
		}
		withArtifact := func(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
			return func(w http.ResponseWriter, request *http.Request) {
				if !store.ValidIdentifier(identifier) {
					writeProblem(w, request, http.StatusBadRequest, "core_artifact_id_invalid", "Core artifact ID invalid", "The core artifact ID is invalid.")
					return
				}
				next(w, request, identifier)
			}
		}
		switch action {
		case "":
			byMethod[http.MethodGet] = withArtifact(handler.getCoreArtifact)
			byMethod[http.MethodDelete] = withArtifact(handler.deleteCoreArtifact)
		case "enable":
			byMethod[http.MethodPost] = withArtifact(handler.enableCoreArtifact)
		case "disable":
			byMethod[http.MethodPost] = withArtifact(handler.disableCoreArtifact)
		case "configuration-support":
			byMethod[http.MethodGet] = withArtifact(handler.coreConfigurationSupport)
		case "configuration-schema":
			byMethod[http.MethodGet] = withArtifact(handler.coreConfigurationSchema)
		default:
			return nil
		}
	}
	if next, ok := byMethod[method]; ok {
		return next
	}
	return methodNotAllowed
}

// matchCoreArtifactRoute splits /core/artifacts/{artifactId}[/{action}].
func matchCoreArtifactRoute(resource string) (string, string, bool) {
	rest, ok := strings.CutPrefix(resource, "/core/artifacts/")
	if !ok {
		return "", "", false
	}
	identifier, action, _ := strings.Cut(rest, "/")
	if identifier == "" || strings.Contains(action, "/") {
		return "", "", false
	}
	return identifier, action, true
}

func requestNodeID(request *http.Request) string { return request.PathValue("nodeId") }

// nodeProblem is how the management API reports a problem code a node
// returns for a command.
type nodeProblem struct {
	status int
	code   string
	title  string
}

var nodeProblems = map[string]nodeProblem{
	"core_artifact_not_found":        {http.StatusNotFound, "core_artifact_not_found", "Core artifact not found"},
	"core_artifact_in_use":           {http.StatusConflict, "core_artifact_in_use", "Core artifact is in use"},
	"core_platform_mismatch":         {http.StatusConflict, "core_enable_blocked", "Core cannot be enabled"},
	"core_not_enabled":               {http.StatusConflict, "core_not_enabled", "Core is not enabled"},
	"core_install_failed":            {http.StatusUnprocessableEntity, "core_install_failed", "Core install failed"},
	"core_check_failed":              {http.StatusUnprocessableEntity, "core_check_failed", "Core check failed"},
	"startup_artifact_not_found":     {http.StatusNotFound, "startup_artifact_not_found", "Startup artifact not found"},
	"startup_artifact_state":         {http.StatusConflict, "startup_artifact_state_invalid", "Startup artifact state invalid"},
	"activation_bundle_not_ready":    {http.StatusConflict, "activation_bundle_not_ready", "Activation bundle not ready"},
	"monitoring_tier_unavailable":    {http.StatusConflict, "monitoring_tier_unavailable", "Monitoring tier unavailable"},
	"no_applied_bundle":              {http.StatusConflict, "no_applied_bundle", "No applied bundle"},
	"no_rollback_bundle":             {http.StatusConflict, "no_rollback_bundle", "No rollback bundle"},
	"runtime_intent_stale":           {http.StatusPreconditionFailed, "rollback_bundle_changed", "Rollback bundle changed"},
	"configuration_changed":          {http.StatusPreconditionFailed, "configuration_changed", "Configuration changed"},
	"configuration_invalid":          {http.StatusUnprocessableEntity, "configuration_invalid", "Configuration invalid"},
	"stale_observation":              {http.StatusServiceUnavailable, "runtime_inspection_unavailable", "Runtime inspection unavailable"},
	"inspection_unavailable":         {http.StatusServiceUnavailable, "runtime_inspection_unavailable", "Runtime inspection unavailable"},
	"filesystem_invalid":             {http.StatusBadRequest, "filesystem_invalid", "Filesystem selection unavailable"},
	"filesystem_forbidden":           {http.StatusForbidden, "filesystem_forbidden", "Filesystem selection unavailable"},
	"filesystem_not_found":           {http.StatusNotFound, "filesystem_not_found", "Filesystem selection unavailable"},
	"filesystem_type_mismatch":       {http.StatusUnprocessableEntity, "filesystem_type_mismatch", "Filesystem selection unavailable"},
	"filesystem_directory_too_large": {http.StatusUnprocessableEntity, "filesystem_directory_too_large", "Filesystem selection unavailable"},
	"core_log_invalid":               {http.StatusBadRequest, "core_log_invalid", "Invalid log file"},
	"core_log_current":               {http.StatusConflict, "core_log_current", "Current log is protected"},
}

// writeNodeProblem reports a failed node command. Failures the node
// describes keep its detail; code names any other failure.
func writeNodeProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	var problem *fleet.CommandProblem
	switch {
	case errors.Is(err, store.ErrNodeNotFound):
		writeFleetProblem(w, request, err)
	case errors.Is(err, fleet.ErrNodeOffline):
		writeProblem(w, request, http.StatusServiceUnavailable, "node_offline", "Node offline", "The node is not connected. Commands are not queued; retry when it is online.")
	case errors.Is(err, fleet.ErrNodeResultUnknown):
		writeProblem(w, request, http.StatusBadGateway, "node_result_unknown", "Node result unknown", "The node's session ended before it answered. Reload its state before retrying.")
	case errors.Is(err, context.DeadlineExceeded):
		writeProblem(w, request, http.StatusGatewayTimeout, "node_timeout", "Node did not answer", "The node did not answer in time. Reload its state before retrying.")
	case errors.As(err, &problem):
		if known, ok := nodeProblems[problem.Code]; ok {
			writeProblem(w, request, known.status, known.code, known.title, problem.Detail)
			return
		}
		writeProblem(w, request, http.StatusBadGateway, code, "Node command failed", problem.Detail)
	default:
		writeConfigurationProblem(w, request, code, err)
	}
}
