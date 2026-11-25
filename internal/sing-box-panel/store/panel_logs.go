// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type PanelLogFilter struct {
	Cursor       *LogCursor
	Limit        int
	Offset       int
	Level        string
	Since, Until *time.Time
	Search       string
	SearchCodes  []string
	// NodeID keeps only the entries about one node.
	NodeID string
	// PanelOnly keeps only the panel's own entries, about no node.
	PanelOnly bool
}

// LogScope selects the entries about one node, or with Panel those about no
// node; the zero scope selects every entry.
type LogScope struct {
	NodeID string
	Panel  bool
}

// clause is the scope's condition on log_entries, or "" for every entry.
func (scope LogScope) clause() (string, []any, error) {
	switch {
	case scope.Panel && scope.NodeID != "":
		return "", nil, invalidQuery("a log scope is the panel or one node, not both")
	case scope.Panel:
		return "COALESCE(json_extract(metadata_json, '$.node_id'), '') = ''", nil, nil
	case scope.NodeID != "":
		return "json_extract(metadata_json, '$.node_id') = ?", []any{scope.NodeID}, nil
	}
	return "", nil, nil
}

// MaximumPanelLogSearchCodes bounds the exact-code alternatives of a search:
// about twice the event names the interface translates, so a one-letter
// search that matches every name stays within it.
const MaximumPanelLogSearchCodes = 256

// ValidatePanelLogSearchCodes bounds the exact-code alternatives to a text search.
func ValidatePanelLogSearchCodes(codes []string) error {
	if len(codes) > MaximumPanelLogSearchCodes {
		return fmt.Errorf("at most %d search codes are allowed", MaximumPanelLogSearchCodes)
	}
	for _, code := range codes {
		if len(code) > MaximumLogCodeBytes || !logCodePattern.MatchString(code) {
			return fmt.Errorf("invalid panel log search code")
		}
	}
	return nil
}

// PanelLogPage is one page of the panel log with the number of entries the
// filter matches.
type PanelLogPage struct {
	Items []LogEntry `json:"items"`
	Next  *LogCursor `json:"next,omitempty"`
	Total int        `json:"total"`
}

func (s *Store) ListPanelLogs(ctx context.Context, filter PanelLogFilter) (PanelLogPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return PanelLogPage{}, err
	}
	if err := validatePageOffset(filter.Offset, filter.Cursor != nil); err != nil {
		return PanelLogPage{}, err
	}
	if err := ValidatePanelLogSearchCodes(filter.SearchCodes); err != nil {
		return PanelLogPage{}, err
	}
	clauses := []string{"1=1"}
	args := []any{}
	if filter.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, filter.Level)
	}
	scope, scopeArgs, err := LogScope{NodeID: filter.NodeID, Panel: filter.PanelOnly}.clause()
	if err != nil {
		return PanelLogPage{}, err
	}
	if scope != "" {
		clauses = append(clauses, scope)
		args = append(args, scopeArgs...)
	}
	if filter.Since != nil {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, formatTime(*filter.Since))
	}
	if filter.Until != nil {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, formatTime(*filter.Until))
	}
	var searchClauses []string
	if filter.Search != "" {
		searchClauses = append(searchClauses, "instr(lower(message), lower(?)) > 0", "instr(lower(code), lower(?)) > 0")
		args = append(args, filter.Search, filter.Search)
	}
	if len(filter.SearchCodes) > 0 {
		placeholders := make([]string, len(filter.SearchCodes))
		for i, code := range filter.SearchCodes {
			placeholders[i] = "?"
			args = append(args, code)
		}
		searchClauses = append(searchClauses, "code IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(searchClauses) > 0 {
		clauses = append(clauses, "("+strings.Join(searchClauses, " OR ")+")")
	}
	where := strings.Join(clauses, " AND ")
	tx, err := s.logs.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PanelLogPage{}, err
	}
	defer tx.Rollback()
	result := PanelLogPage{Items: []LogEntry{}}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM log_entries WHERE `+where, args...).Scan(&result.Total); err != nil {
		return PanelLogPage{}, fmt.Errorf("count panel logs: %w", err)
	}
	if filter.Cursor != nil {
		where += " AND (occurred_at < ? OR (occurred_at = ? AND id < ?))"
		args = append(args, formatTime(filter.Cursor.Time), formatTime(filter.Cursor.Time), filter.Cursor.ID)
	}
	args = append(args, limit+1, filter.Offset)
	rows, err := tx.QueryContext(ctx, `SELECT id, occurred_at, source, level, code, message, metadata_json
           FROM log_entries WHERE `+where+` ORDER BY occurred_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return PanelLogPage{}, fmt.Errorf("query panel log: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		entry, err := scanLogEntry(rows)
		if err != nil {
			return PanelLogPage{}, err
		}
		result.Items = append(result.Items, entry)
	}
	if err := rows.Err(); err != nil {
		return PanelLogPage{}, err
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		last := result.Items[limit-1]
		result.Next = &LogCursor{Time: last.Time, ID: last.ID}
	}
	return result, tx.Commit()
}
