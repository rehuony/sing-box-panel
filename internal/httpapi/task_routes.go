// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) listTasks(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	page, err := handler.commands.ListTasks(request.Context(), application.TaskListFilter{
		Lane:   store.TaskLane(request.URL.Query().Get("lane")),
		Status: store.TaskStatus(request.URL.Query().Get("status")),
		Kind:   request.URL.Query().Get("kind"),
		Limit:  limit,
	})
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "task_filter_invalid", "Task filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) getTask(w http.ResponseWriter, request *http.Request, taskID string) {
	if !handler.requireCommands(w, request) {
		return
	}
	task, err := handler.commands.Task(request.Context(), taskID)
	if err != nil {
		writeTaskProblem(w, request, "task_read_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (handler *Handler) cancelTask(w http.ResponseWriter, request *http.Request, taskID string) {
	if !handler.requireCommands(w, request) {
		return
	}
	task, err := handler.commands.CancelTask(request.Context(), taskID)
	if err != nil {
		writeTaskProblem(w, request, "task_cancel_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func decodeStrictRequest(w http.ResponseWriter, request *http.Request, limit int64, target any) bool {
	raw, err := readBoundedBody(request, limit)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "request_too_large", "Request rejected", err.Error())
		return false
	}
	if err := jsonstrict.Decode(raw, limit, target); err != nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "invalid_json", "Request invalid", err.Error())
		return false
	}
	return true
}

func optionalPositiveInt64(w http.ResponseWriter, request *http.Request, name string) (int64, bool) {
	raw := request.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", name+" must be a positive integer.")
		return 0, false
	}
	return value, true
}

func optionalLimit(w http.ResponseWriter, request *http.Request) (int, bool) {
	raw := request.URL.Query().Get("limit")
	if raw == "" {
		return 50, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 200 {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "limit must be between 1 and 200.")
		return 0, false
	}
	return value, true
}

func writeCanonicalSave(w http.ResponseWriter, status int, result application.CanonicalSave) {
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, status, result)
}

func writeCanonicalProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsRevisionConflict(err):
		writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
	case errors.Is(err, configuration.ErrInvalidDocument):
		writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_invalid", "Configuration invalid", err.Error())
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Configuration operation failed", "The configuration operation could not be completed.")
	}
}

func writeRevisionProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	if application.IsRevisionNotFound(err) {
		writeProblem(w, request, http.StatusNotFound, "revision_not_found", "Revision not found", err.Error())
		return
	}
	writeProblem(w, request, http.StatusInternalServerError, code, "Revision operation failed", "The revision operation could not be completed.")
}

func writeTaskProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	if application.IsTaskNotFound(err) {
		writeProblem(w, request, http.StatusNotFound, "task_not_found", "Task not found", err.Error())
		return
	}
	writeProblem(w, request, http.StatusInternalServerError, code, "Task operation failed", "The task operation could not be completed.")
}
