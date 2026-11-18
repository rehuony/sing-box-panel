// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func decodeStrictRequest(w http.ResponseWriter, request *http.Request, limit int64, target any) bool {
	raw, err := readBoundedBody(request, limit)
	if err != nil {
		writeBodyProblem(w, request, err)
		return false
	}
	if err := jsonstrict.Decode(raw, limit, target); err != nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "invalid_json", "Request invalid", err.Error())
		return false
	}
	return true
}

// writeQueryProblem answers a failed read or clear of recorded data: a
// filter, range or cursor the caller supplied as 400 with code and its
// description, any other failure as 503 without the storage error's text.
func writeQueryProblem(w http.ResponseWriter, request *http.Request, code, title string, err error) {
	if errors.Is(err, store.ErrInvalidQuery) {
		writeProblem(w, request, http.StatusBadRequest, code, title, err.Error())
		return
	}
	writeProblem(w, request, http.StatusServiceUnavailable, "records_unavailable", "Records unavailable", "The panel could not read its records. Retry later.")
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
