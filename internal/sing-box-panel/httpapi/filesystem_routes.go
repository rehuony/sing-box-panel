// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
)

func (handler *Handler) listFilesystemEntries(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "path", "search", "show_hidden", "offset", "limit")
	if !ok {
		return
	}
	hidden, ok := optionalStrictBool(w, request, query, "show_hidden")
	if !ok {
		return
	}
	input := fleet.PathQuery{Path: query.Get("path"), Search: query.Get("search"), ShowHidden: hidden, Limit: 50}
	for name, target := range map[string]*int{"offset": &input.Offset, "limit": &input.Limit} {
		if query.Has(name) {
			value, err := strconv.ParseInt(query.Get(name), 10, 32)
			if err != nil || value < 0 || (name == "limit" && (value < 1 || value > 200)) {
				writeInvalidFilesystemQuery(w, request)
				return
			}
			*target = int(value)
		}
	}
	page, err := handler.services.Fleet.Browse(request.Context(), requestNodeID(request), input)
	if err != nil {
		writeNodeProblem(w, request, "filesystem_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) resolveFilesystemPath(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "path", "mode")
	if !ok {
		return
	}
	switch query.Get("mode") {
	case "file", "directory", "output-file", "socket":
	default:
		writeInvalidFilesystemQuery(w, request)
		return
	}
	selection, err := handler.services.Fleet.Resolve(request.Context(), requestNodeID(request), query.Get("path"), query.Get("mode"))
	if err != nil {
		writeNodeProblem(w, request, "filesystem_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, selection)
}

func writeInvalidFilesystemQuery(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusBadRequest, "filesystem_invalid", "Filesystem selection unavailable", "Check the path, selection mode and pagination parameters.")
}
