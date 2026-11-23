// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

const maximumNodeRequestBytes = 4 << 10

func (handler *Handler) fleetRequest(w http.ResponseWriter, request *http.Request) bool {
	_, ok := strictCoreQuery(w, request)
	return ok
}

type nodeNameRequest struct {
	Name string `json:"name"`
}

func (handler *Handler) listNodes(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	nodes, err := handler.services.Fleet.Nodes(request.Context())
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []fleet.Node `json:"items"`
	}{nodes})
}

// Bounds of the fleet status filters.
const (
	maximumFleetPageLimit  = 1000
	maximumFleetQueryRunes = 256
)

// fleetStatusFilter reads the filters of a fleet status read or stream, and
// with paged the page's limit and cursor, or writes the problem.
func (handler *Handler) fleetStatusFilter(w http.ResponseWriter, request *http.Request, paged bool) (fleet.StatusFilter, int, string, bool) {
	names := []string{"state", "tag", "q"}
	if paged {
		names = append(names, "limit", "cursor")
	}
	query, ok := strictCoreQuery(w, request, names...)
	if !ok {
		return fleet.StatusFilter{}, 0, "", false
	}
	filter := fleet.StatusFilter{State: query.Get("state"), Tag: query.Get("tag"), Query: query.Get("q")}
	invalid := func(detail string) (fleet.StatusFilter, int, string, bool) {
		writeProblem(w, request, http.StatusBadRequest, "fleet_filter_invalid", "Fleet filter invalid", detail)
		return fleet.StatusFilter{}, 0, "", false
	}
	switch filter.State {
	case "", fleet.StateOnline, fleet.StateOffline:
	default:
		return invalid("state must be online or offline.")
	}
	if filter.Tag != "" && !store.ValidNodeTag(filter.Tag) {
		return invalid("tag must be 1 to 32 lowercase letters, digits, hyphens or underscores.")
	}
	if utf8.RuneCountInString(filter.Query) > maximumFleetQueryRunes {
		return invalid("q must be at most 256 characters.")
	}
	limit := 0
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maximumFleetPageLimit {
			return invalid("limit must be between 1 and 1000.")
		}
		limit = value
	}
	cursor := query.Get("cursor")
	if cursor != "" && !fleet.ValidStatusCursor(cursor) {
		return invalid(fleet.ErrStatusCursorInvalid.Error())
	}
	return filter, limit, cursor, true
}

// fleetStatus answers with the shared fleet snapshot while it is fresh. Its
// ETag is the response's digest, so a client holding the same page gets 304.
func (handler *Handler) fleetStatus(w http.ResponseWriter, request *http.Request) {
	filter, limit, cursor, ok := handler.fleetStatusFilter(w, request, true)
	if !ok {
		return
	}
	snapshot, err := handler.services.Fleet.Status(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "fleet_status_unavailable", "Fleet status unavailable", "The nodes' metrics could not be read.")
		return
	}
	page, err := snapshot.Page(filter, limit, cursor)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "fleet_filter_invalid", "Fleet filter invalid", err.Error())
		return
	}
	body, err := json.Marshal(page)
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "fleet_status_unavailable", "Fleet status unavailable", "The nodes' metrics could not be encoded.")
		return
	}
	digest := sha256.Sum256(body)
	etag := quoteETag(hex.EncodeToString(digest[:16]))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if noneMatchHits(request.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(body, '\n'))
}

// fleetStreamLifetime ends each fleet stream so the client reauthenticates.
const fleetStreamLifetime = time.Minute

// maximumFleetFrameBytes bounds one frame of the fleet stream, whose first
// frame carries every node it selects.
const maximumFleetFrameBytes = 16 << 20

// streamFleetStatus streams the nodes the filter selects: all of them first,
// then, from the shared snapshot read every two seconds, only the nodes that
// changed and the ones that went away.
func (handler *Handler) streamFleetStatus(w http.ResponseWriter, request *http.Request) {
	filter, _, _, ok := handler.fleetStatusFilter(w, request, false)
	if !ok {
		return
	}
	changes := fleet.NewStatusChanges(filter)
	streamSnapshots(w, request, "fleet", fleetStreamLifetime, maximumFleetFrameBytes, func(context.Context) (<-chan telemetry.SnapshotResult[fleet.FleetSnapshot], func()) {
		return handler.services.Fleet.SubscribeStatus()
	}, func(snapshot fleet.FleetSnapshot) ([]byte, error) {
		frame, changed := changes.Next(snapshot)
		if !changed {
			return nil, nil
		}
		return json.Marshal(frame)
	})
}

// fleetCommandRequest is one command for many nodes: listed by ID, or every
// node with a tag.
type fleetCommandRequest struct {
	Action       string   `json:"action"`
	NodeIDs      []string `json:"node_ids"`
	Tag          string   `json:"tag"`
	ExactVersion string   `json:"exact_version"`
}

// fleetCommandResult is one node's outcome; a failure carries the problem
// code and title the single-node operation answers with.
type fleetCommandResult struct {
	NodeID string `json:"node_id"`
	OK     bool   `json:"ok"`
	// Skipped marks a node a sequential update did not reach.
	Skipped bool   `json:"skipped,omitempty"`
	Code    string `json:"code,omitempty"`
	Title   string `json:"title,omitempty"`
}

// fleetCommandSummary ends a fleet command's stream.
type fleetCommandSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// Bounds of a fleet command request.
const (
	maximumFleetCommandNodes        = 1000
	maximumFleetCommandRequestBytes = 64 << 10
)

// fleetCommands runs one action on many nodes, a few at once, and streams
// every node's outcome as it finishes, so progress and partial success are
// visible. Problems found before any node runs answer as a problem.
func (handler *Handler) fleetCommands(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input fleetCommandRequest
	if !decodeStrictRequest(w, request, maximumFleetCommandRequestBytes, &input) {
		return
	}
	invalid := func(detail string) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "fleet_command_invalid", "Fleet command invalid", detail)
	}
	if (len(input.NodeIDs) == 0) == (input.Tag == "") {
		invalid("Give either node_ids or tag.")
		return
	}
	if len(input.NodeIDs) > maximumFleetCommandNodes {
		invalid("node_ids lists at most 1000 nodes.")
		return
	}
	for _, nodeID := range input.NodeIDs {
		if !store.ValidIdentifier(nodeID) {
			invalid("node_ids contains an invalid node ID.")
			return
		}
	}
	if input.Tag != "" && !store.ValidNodeTag(input.Tag) {
		invalid("tag must be 1 to 32 lowercase letters, digits, hyphens or underscores.")
		return
	}
	if (input.Action == "install_core") != (input.ExactVersion != "") {
		invalid("exact_version is required for install_core and not allowed for other actions.")
		return
	}
	var command func(context.Context, fleet.Node) error
	failure := "runtime_" + input.Action + "_failed"
	run := handler.services.Fleet.RunBatch
	switch input.Action {
	case "start":
		command = func(ctx context.Context, node fleet.Node) error {
			_, err := handler.services.Fleet.StartRuntime(ctx, node.ID)
			return err
		}
	case "stop":
		command = func(ctx context.Context, node fleet.Node) error {
			_, err := handler.services.Fleet.StopRuntime(ctx, node.ID)
			return err
		}
	case "restart":
		command = func(ctx context.Context, node fleet.Node) error {
			_, err := handler.services.Fleet.RestartRuntime(ctx, node.ID)
			return err
		}
	case "install_core":
		var ok bool
		if command, ok = handler.versionInstaller(w, request, input.ExactVersion); !ok {
			return
		}
		failure = "core_install_failed"
	case "update":
		// Updates run one node at a time: each must come back with the
		// panel's version before the next starts, and a failure stops them.
		if _, err := handler.services.Releases.NodeVersion(); err != nil {
			nodeUpdateProblem(err).write(w, request)
			return
		}
		command = func(ctx context.Context, node fleet.Node) error {
			return handler.services.Fleet.UpdateNodeAndWait(ctx, node.ID)
		}
		run = handler.services.Fleet.RunSequential
	default:
		invalid("action must be start, stop, restart, install_core or update.")
		return
	}
	nodes, err := handler.services.Fleet.BatchNodes(request.Context(), fleet.BatchTarget{NodeIDs: input.NodeIDs, Tag: input.Tag})
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	handler.streamBatch(w, request, nodes, run, command, func(err error) problemDetails {
		if problem, known := batchInstallProblem(err); known {
			return problem
		}
		if input.Action == "update" {
			return nodeUpdateProblem(err)
		}
		return nodeCommandProblem(failure, err)
	})
}

// batchRunner runs a batch command on nodes and reports each outcome.
type batchRunner func(context.Context, []fleet.BatchNode, func(context.Context, fleet.Node) error, func(fleet.BatchResult))

// streamBatch runs command on nodes and streams a result event per node as
// it finishes, then a done event. The batch runs to its end when the client
// leaves; heartbeats keep an idle connection open meanwhile.
func (handler *Handler) streamBatch(w http.ResponseWriter, request *http.Request, nodes []fleet.BatchNode,
	run batchRunner, command func(context.Context, fleet.Node) error, problem func(error) problemDetails,
) {
	stream, ok := newEventStream(request.Context(), w, time.Time{}, maximumSnapshotFrameBytes)
	if !ok {
		writeProblem(w, request, http.StatusInternalServerError, "stream_unavailable", "Stream unavailable", "The transport does not support streaming.")
		return
	}
	results := make(chan fleet.BatchResult, len(nodes))
	go func() {
		defer close(results)
		run(request.Context(), nodes, command, func(result fleet.BatchResult) { results <- result })
	}()
	// The client keeps reading only while every write succeeds.
	connected := stream.open() == nil
	send := func(name string, value any) {
		if !connected {
			return
		}
		data, err := json.Marshal(value)
		connected = err == nil && stream.event(name, data) == nil
	}
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	summary := fleetCommandSummary{Total: len(nodes)}
	for {
		select {
		case result, more := <-results:
			if !more {
				send("done", summary)
				return
			}
			item := fleetCommandResult{NodeID: result.NodeID, OK: result.Err == nil}
			switch {
			case result.Err == nil:
				summary.Succeeded++
			case errors.Is(result.Err, fleet.ErrSkipped):
				item.Skipped = true
				summary.Skipped++
			default:
				summary.Failed++
				failure := problem(result.Err)
				item.Code, item.Title = failure.code, failure.title
			}
			send("result", item)
		case <-heartbeat.C:
			connected = connected && stream.heartbeat() == nil
		}
	}
}

func batchInstallProblem(err error) (problemDetails, bool) {
	switch {
	case errors.Is(err, fleet.ErrNodePlatformUnknown):
		return problemDetails{http.StatusConflict, "node_platform_unknown", "Node platform unknown", err.Error()}, true
	case errors.Is(err, fleet.ErrCoreAssetUnavailable):
		return problemDetails{http.StatusConflict, "core_asset_unavailable", "Core archive unavailable", err.Error()}, true
	case errors.Is(err, fleet.ErrCoreDownloadFailed):
		return problemDetails{http.StatusBadGateway, "core_download_failed", "Core download failed", err.Error()}, true
	}
	return problemDetails{}, false
}

// versionInstaller returns the fleet's command that installs exactVersion on
// each node, or writes the problem when it cannot be prepared.
func (handler *Handler) versionInstaller(w http.ResponseWriter, request *http.Request, exactVersion string) (func(context.Context, fleet.Node) error, bool) {
	if _, err := coreartifact.ParseExactVersion(exactVersion); err != nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_install_invalid", "Core install request invalid", "exact_version must be an exact sing-box version.")
		return nil, false
	}
	install, err := handler.services.Fleet.VersionInstaller(request.Context(), exactVersion)
	switch {
	case corecache.IsCatalogNotInitialized(err):
		writeProblem(w, request, http.StatusConflict, "catalog_not_initialized", "Catalog not initialized", "Refresh the core catalog before installing an asset.")
		return nil, false
	case errors.Is(err, fleet.ErrVersionNotInCatalog):
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_install_invalid", "Core install request invalid", "The catalog has no official archive of this version.")
		return nil, false
	case err != nil:
		writeProblem(w, request, http.StatusInternalServerError, "core_install_failed", "Core install failed", "The catalog could not be inspected before installation.")
		return nil, false
	}
	return install, true
}

func (handler *Handler) createNode(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input nodeNameRequest
	if !decodeStrictRequest(w, request, maximumNodeRequestBytes, &input) {
		return
	}
	enrollment, err := handler.services.Fleet.AddNode(request.Context(), input.Name)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeEnrollment(w, http.StatusCreated, enrollment)
}

func (handler *Handler) getNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	node, err := handler.services.Fleet.Node(request.Context(), id)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (handler *Handler) updateNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input nodeNameRequest
	if !decodeStrictRequest(w, request, maximumNodeRequestBytes, &input) {
		return
	}
	node, err := handler.services.Fleet.RenameNode(request.Context(), id, input.Name)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (handler *Handler) deleteNode(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	if err := handler.services.Fleet.DeleteNode(request.Context(), id); err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) createNodeEnrollment(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	enrollment, err := handler.services.Fleet.IssueEnrollment(request.Context(), id)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	writeEnrollment(w, http.StatusCreated, enrollment)
}

// cancelNodeEnrollment discards the node ID's unused code; a node ID without
// one succeeds as well.
func (handler *Handler) cancelNodeEnrollment(w http.ResponseWriter, request *http.Request, id string) {
	if !handler.fleetRequest(w, request) {
		return
	}
	if err := handler.services.Fleet.CancelEnrollment(request.Context(), id); err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeEnrollment returns a one-time code, which must never be cached.
func writeEnrollment(w http.ResponseWriter, statusCode int, enrollment fleet.Enrollment) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, statusCode, enrollment)
}

func writeFleetProblem(w http.ResponseWriter, request *http.Request, err error) {
	fleetProblem(err).write(w, request)
}

func fleetProblem(err error) problemDetails {
	switch {
	case errors.Is(err, store.ErrNodeNotFound):
		return problemDetails{http.StatusNotFound, "node_not_found", "Node not found", "The node does not exist."}
	case errors.Is(err, store.ErrNodeNameConflict):
		return problemDetails{http.StatusConflict, "node_name_conflict", "Node name in use", "Another node already uses this name."}
	case errors.Is(err, fleet.ErrNameInvalid):
		return problemDetails{http.StatusUnprocessableEntity, "node_name_invalid", "Node name invalid", err.Error()}
	case errors.Is(err, store.ErrNodeSettingsConflict):
		return problemDetails{http.StatusPreconditionFailed, "node_settings_conflict", "Node settings changed", "Reload the node's settings before saving."}
	case errors.Is(err, store.ErrNodeSettingsInvalid):
		return problemDetails{http.StatusUnprocessableEntity, "node_settings_invalid", "Node settings invalid", err.Error()}
	case errors.Is(err, fleet.ErrEndpointRequired):
		return problemDetails{http.StatusConflict, "node_endpoint_required", "Node endpoint required", err.Error()}
	default:
		return problemDetails{http.StatusInternalServerError, "node_operation_failed", "Node operation failed", "The node operation could not be completed."}
	}
}

func (handler *Handler) nodeSettings(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	result, err := handler.services.Fleet.NodeSettings(request.Context(), requestNodeID(request))
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) saveNodeSettings(w http.ResponseWriter, request *http.Request) {
	if !handler.fleetRequest(w, request) {
		return
	}
	var input fleet.NodeSettings
	if !decodeStrictRequest(w, request, 4096, &input) {
		return
	}
	result, err := handler.services.Fleet.SaveNodeSettings(request.Context(), requestNodeID(request), input)
	if err != nil {
		writeFleetProblem(w, request, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}
