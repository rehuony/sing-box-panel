// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
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
	Level        string
	Since, Until *time.Time
	Search       string
}
type PanelLogPage struct {
	Items []PanelLog `json:"items"`
	Next  *LogCursor `json:"next,omitempty"`
}

func (s *Store) ListPanelLogs(ctx context.Context, filter PanelLogFilter) (PanelLogPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return PanelLogPage{}, err
	}
	clauses := []string{"1=1"}
	args := []any{}
	if filter.Cursor != nil {
		clauses = append(clauses, "(occurred_at < ? OR (occurred_at = ? AND id < ?))")
		args = append(args, formatTime(filter.Cursor.Time), formatTime(filter.Cursor.Time), filter.Cursor.ID)
	}
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
	if filter.Search != "" {
		clauses = append(clauses, "(instr(lower(message), lower(?)) > 0 OR instr(lower(code), lower(?)) > 0)")
		args = append(args, filter.Search, filter.Search)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `WITH combined AS (
 SELECT 'log:'||id AS id,occurred_at,source,level,code,message,'' AS status,metadata_json FROM log_entries
 UNION ALL
 SELECT 'runtime:'||id,occurred_at,'runtime',CASE WHEN state='failed' THEN 'error' ELSE 'info' END,reason,reason,state,'{}' FROM runtime_transitions
 ) SELECT id,occurred_at,source,level,code,message,status,metadata_json FROM combined WHERE `+strings.Join(clauses, " AND ")+` ORDER BY occurred_at DESC,id DESC LIMIT ?`, args...)

	if err != nil {
		return PanelLogPage{}, fmt.Errorf("query panel log: %w", err)
	}
	defer rows.Close()
	result := PanelLogPage{Items: []PanelLog{}}
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
	return result, nil
}
