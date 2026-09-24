// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/filesystem"
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
	input := filesystem.Query{Path: query.Get("path"), Search: query.Get("search"), ShowHidden: hidden, Limit: 50}
	for name, target := range map[string]*int{"offset": &input.Offset, "limit": &input.Limit} {
		if query.Has(name) {
			value, err := strconv.ParseInt(query.Get(name), 10, 32)
			if err != nil || value < 0 || (name == "limit" && (value < 1 || value > 200)) {
				writeFilesystemProblem(w, request, filesystem.ErrInvalid)
				return
			}
			*target = int(value)
		}
	}
	page, err := handler.commands.ListFilesystemEntries(request.Context(), input)
	if err != nil {
		writeFilesystemProblem(w, request, err)
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
	selection, err := handler.commands.ResolveFilesystemPath(request.Context(), query.Get("path"), filesystem.Mode(query.Get("mode")))
	if err != nil {
		writeFilesystemProblem(w, request, err)
		return
	}
	writeJSON(w, http.StatusOK, selection)
}

func writeFilesystemProblem(w http.ResponseWriter, request *http.Request, err error) {
	status, code, detail := http.StatusInternalServerError, "filesystem_failed", "The filesystem operation could not be completed."
	switch {
	case errors.Is(err, filesystem.ErrInvalid):
		status, code, detail = http.StatusBadRequest, "filesystem_invalid", "Check the path, selection mode and pagination parameters."
	case errors.Is(err, fs.ErrPermission):
		status, code, detail = http.StatusForbidden, "filesystem_forbidden", "The panel process cannot access this path."
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, filesystem.ErrBrokenLink):
		status, code, detail = http.StatusNotFound, "filesystem_not_found", "The path or symbolic link target no longer exists."
	case errors.Is(err, filesystem.ErrType):
		status, code, detail = http.StatusUnprocessableEntity, "filesystem_type_mismatch", "The target has the wrong type for this field."
	case errors.Is(err, filesystem.ErrTooLarge):
		status, code, detail = http.StatusUnprocessableEntity, "filesystem_directory_too_large", "This directory exceeds the 20000-entry browsing limit. Enter a subdirectory path directly."
	}
	writeProblem(w, request, status, code, "Filesystem selection unavailable", detail)
}
