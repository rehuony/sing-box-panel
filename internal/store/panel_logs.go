// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type PanelLog struct {
	ID       string          `json:"id"`
	Time     time.Time       `json:"time"`
	Source   string          `json:"source"`
	Level    string          `json:"level"`
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Status   string          `json:"status"`
	Metadata json.RawMessage `json:"metadata"`
}
type PanelLogFilter struct {
	Cursor       *LogCursor
	Limit        int
	Offset       int
	Level        string
	Since, Until *time.Time
	Search       string
	SearchCodes  []string
}

const MaximumPanelLogSearchCodes = 128

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

type PanelLogPage struct {
	Items []PanelLog `json:"items"`
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
	const combined = `WITH combined AS (
 SELECT 'log:'||id AS id,occurred_at,source,level,code,message,'' AS status,metadata_json FROM log_entries
 UNION ALL
 SELECT 'runtime:'||id,occurred_at,'runtime',CASE WHEN state='failed' THEN 'error' ELSE 'info' END,reason,reason,state,
 json_patch('{}', json_object('pid',pid,'process_started_at',process_started_at,
 'generation',generation,'activation_bundle_id',activation_bundle_id,'uncertain_since',uncertain_since))
 FROM runtime_transitions
 ) `
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PanelLogPage{}, err
	}
	defer tx.Rollback()
	result := PanelLogPage{Items: []PanelLog{}}
	if err := tx.QueryRowContext(ctx, combined+`SELECT COUNT(*) FROM combined WHERE `+strings.Join(clauses, " AND "), args...).Scan(&result.Total); err != nil {
		return PanelLogPage{}, fmt.Errorf("count panel logs: %w", err)
	}
	if filter.Cursor != nil {
		clauses = append(clauses, "(occurred_at < ? OR (occurred_at = ? AND id < ?))")
		args = append(args, formatTime(filter.Cursor.Time), formatTime(filter.Cursor.Time), filter.Cursor.ID)
	}
	args = append(args, limit+1, filter.Offset)
	rows, err := tx.QueryContext(ctx, combined+`SELECT id,occurred_at,source,level,code,message,status,metadata_json FROM combined WHERE `+strings.Join(clauses, " AND ")+` ORDER BY occurred_at DESC,id DESC LIMIT ? OFFSET ?`, args...)

	if err != nil {
		return PanelLogPage{}, fmt.Errorf("query panel log: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item PanelLog
		var at, metadata string
		if err := rows.Scan(&item.ID, &at, &item.Source, &item.Level, &item.Code, &item.Message, &item.Status, &metadata); err != nil {
			return PanelLogPage{}, err
		}
		item.Time, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return PanelLogPage{}, err
		}
		item.Metadata = json.RawMessage(metadata)
		result.Items = append(result.Items, item)
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
