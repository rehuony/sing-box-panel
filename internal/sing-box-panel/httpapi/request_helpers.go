// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

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

func (handler *Handler) requireCommands(w http.ResponseWriter, request *http.Request) bool {
	if handler.services.Fleet != nil {
		return true
	}
	writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Configuration services are not ready.")
	return false
}
