// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (handler *Handler) listStartupArtifacts(w http.ResponseWriter, request *http.Request) {
	query, ok := strictCoreQuery(
		w, request,
		"canonical_revision_id", "core_version", "core_artifact_id", "state",
		"before_time", "before_id", "limit",
	)
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("core_version"), true) ||
		(query.Get("canonical_revision_id") != "" && !store.ValidIdentifier(query.Get("canonical_revision_id"))) ||
		(query.Get("core_artifact_id") != "" && !store.ValidIdentifier(query.Get("core_artifact_id"))) {
		writeProblem(w, request, http.StatusBadRequest, "startup_artifact_filter_invalid", "Startup artifact filter invalid", "The startup artifact filter contains an invalid identity or exact version.")
		return
	}
	state := query.Get("state")
	if state != "" && state != "pending" && state != "ready" && state != "failed" {
		writeProblem(w, request, http.StatusBadRequest, "startup_artifact_filter_invalid", "Startup artifact filter invalid", "state must be pending, ready, or failed.")
		return
	}
	cursor, ok := startupArtifactCursor(w, request, query.Get("before_time"), query.Get("before_id"))
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	page, err := handler.services.Fleet.ListStartupArtifacts(request.Context(), requestNodeID(request), fleet.StartupArtifactListRequest{
		CanonicalRevisionID: query.Get("canonical_revision_id"), ExactCoreVersion: query.Get("core_version"),
		CoreArtifactID: query.Get("core_artifact_id"), State: state, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		writeNodeProblem(w, request, "startup_artifact_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func startupArtifactCursor(
	w http.ResponseWriter,
	request *http.Request,
	rawTime string,
	identifier string,
) (*fleet.StartupArtifactCursor, bool) {
	if rawTime == "" && identifier == "" {
		return nil, true
	}
	if rawTime == "" || identifier == "" || !store.ValidIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time and before_id must be supplied together.")
		return nil, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(rawTime))
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time must be an RFC 3339 timestamp.")
		return nil, false
	}
	return &fleet.StartupArtifactCursor{CreatedAt: createdAt.UTC(), ID: identifier}, true
}
