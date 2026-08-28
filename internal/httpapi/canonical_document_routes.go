// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

func (handler *Handler) canonicalDocument(w http.ResponseWriter, request *http.Request) {
	if handler.commands == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Canonical configuration services are not ready.")
		return
	}
	head, err := handler.commands.CanonicalHead(request.Context())
	if err != nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "canonical_read_failed", "Configuration unavailable", "The canonical revision could not be loaded.")
		return
	}
	if head == nil {
		writeProblem(w, request, http.StatusNotFound, "canonical_not_initialized", "Configuration not initialized", "No canonical revision has been saved.")
		return
	}
	w.Header().Set("ETag", quoteETag(head.ID))
	writeJSON(w, http.StatusOK, head)
}

func (handler *Handler) replaceCanonicalDocument(w http.ResponseWriter, request *http.Request) {
	if handler.commands == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Canonical configuration services are not ready.")
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	raw, err := readBoundedBody(request, configuration.MaximumBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "canonical_too_large", "Configuration rejected", err.Error())
		return
	}
	result, err := handler.commands.ReplaceCanonical(request.Context(), expectedHead, raw)
	if err != nil {
		switch {
		case application.IsRevisionConflict(err):
			writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
		case errors.Is(err, configuration.ErrInvalidDocument):
			writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_invalid", "Configuration invalid", err.Error())
		default:
			writeProblem(w, request, http.StatusInternalServerError, "canonical_save_failed", "Configuration save failed", "The canonical revision could not be saved.")
		}
		return
	}
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) patchCanonicalDocument(w http.ResponseWriter, request *http.Request) {
	if handler.commands == nil {
		writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Canonical configuration services are not ready.")
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	raw, err := readBoundedBody(request, maxCanonicalPatchBody)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "canonical_patch_too_large", "Configuration patch rejected", err.Error())
		return
	}
	var input struct {
		Changes []application.CanonicalChange `json:"changes"`
	}
	if err := jsonstrict.Decode(raw, maxCanonicalPatchBody, &input); err != nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_patch_invalid", "Configuration patch invalid", err.Error())
		return
	}
	result, err := handler.commands.PatchCanonical(request.Context(), expectedHead, input.Changes)
	if err != nil {
		switch {
		case application.IsRevisionConflict(err):
			writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
		case errors.Is(err, application.ErrCanonicalPatchInvalid):
			writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_patch_invalid", "Configuration patch invalid", err.Error())
		default:
			writeProblem(w, request, http.StatusInternalServerError, "canonical_patch_failed", "Configuration patch failed", "The canonical changes could not be saved.")
		}
		return
	}
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, http.StatusOK, result)
}

func parseIfMatch(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' || strings.HasPrefix(value, "W/") {
		return "", errors.New(`If-Match must contain one quoted revision ID, or "none" for the first revision`)
	}
	identifier := value[1 : len(value)-1]
	if identifier == "none" {
		return "", nil
	}
	if len(identifier) > 256 || identifier == "" || strings.ContainsAny(identifier, "\"\\,\r\n") {
		return "", errors.New("If-Match contains an invalid revision ID")
	}
	return identifier, nil
}

func quoteETag(identifier string) string {
	return `"` + identifier + `"`
}
