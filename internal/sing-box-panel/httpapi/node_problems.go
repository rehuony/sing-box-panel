// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

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
	"core_schema_unavailable":        {http.StatusConflict, "configuration_schema_unavailable", "Configuration schema unavailable"},
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
	"update_unsupported":             {http.StatusConflict, "node_update_unsupported", "Node cannot update itself"},
	"update_permission_denied":       {http.StatusConflict, "node_update_permission_denied", "Node cannot replace its binary"},
	"update_in_progress":             {http.StatusConflict, "node_update_in_progress", "Node update in progress"},
}

// writeNodeProblem reports a failed node command. Failures the node
// describes keep its detail; code names any other failure.
func writeNodeProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	nodeCommandProblem(code, err).write(w, request)
}

func nodeCommandProblem(code string, err error) problemDetails {
	var problem *fleet.CommandProblem
	switch {
	case errors.Is(err, store.ErrNodeNotFound):
		return fleetProblem(err)
	case errors.Is(err, fleet.ErrNodeOffline):
		return problemDetails{http.StatusServiceUnavailable, "node_offline", "Node offline", "The node is not connected. Commands are not queued; retry when it is online."}
	case errors.Is(err, fleet.ErrNodeUpgradeRequired):
		return problemDetails{http.StatusConflict, "node_upgrade_required", "Node upgrade required", err.Error()}
	case errors.Is(err, fleet.ErrNodeResultUnknown):
		return problemDetails{http.StatusServiceUnavailable, "node_result_unknown", "Node result unknown", "The node's session ended before it answered. Reload its state before retrying."}
	case errors.Is(err, context.DeadlineExceeded):
		return problemDetails{http.StatusServiceUnavailable, "node_timeout", "Node did not answer", "The node did not answer in time. Reload its state before retrying."}
	case errors.As(err, &problem):
		if known, ok := nodeProblems[problem.Code]; ok {
			return problemDetails{known.status, known.code, known.title, problem.Detail}
		}
		return problemDetails{http.StatusServiceUnavailable, code, "Node command failed", problem.Detail}
	default:
		return configurationProblem(code, err)
	}
}
