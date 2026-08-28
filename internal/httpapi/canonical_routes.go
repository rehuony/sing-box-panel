// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"

	"github.com/rehuony/sing-box-panel/internal/application"
)

func (handler *Handler) listRevisions(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	before, ok := optionalPositiveInt64(w, request, "before_sequence")
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	page, err := handler.commands.ListCanonicalRevisions(request.Context(), before, limit)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "revision_filter_invalid", "Revision filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) getRevision(w http.ResponseWriter, request *http.Request, reference string) {
	if !handler.requireCommands(w, request) {
		return
	}
	revision, err := handler.commands.CanonicalRevision(request.Context(), reference)
	if err != nil {
		writeRevisionProblem(w, request, "revision_read_failed", err)
		return
	}
	w.Header().Set("ETag", quoteETag(revision.ID))
	writeJSON(w, http.StatusOK, revision)
}

func (handler *Handler) diffRevisions(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	from, to := request.URL.Query().Get("from"), request.URL.Query().Get("to")
	if from == "" || to == "" {
		writeProblem(w, request, http.StatusBadRequest, "revision_diff_references_required", "Revision references required", "Both from and to query parameters are required.")
		return
	}
	diff, err := handler.commands.DiffCanonicalRevisions(request.Context(), from, to)
	if err != nil {
		writeRevisionProblem(w, request, "revision_diff_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (handler *Handler) restoreRevision(w http.ResponseWriter, request *http.Request, reference string) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	result, err := handler.commands.RestoreCanonicalRevision(request.Context(), expectedHead, reference)
	if err != nil {
		if application.IsRevisionConflict(err) {
			writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
			return
		}
		writeRevisionProblem(w, request, "revision_restore_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusOK, result)
}
