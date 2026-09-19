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
	TaskID   string          `json:"task_id,omitempty"`
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
		args = append(args, formatTaskTime(filter.Cursor.Time), formatTaskTime(filter.Cursor.Time), filter.Cursor.ID)
	}
	if filter.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, filter.Level)
	}
	if filter.Since != nil {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, formatTaskTime(*filter.Since))
	}
	if filter.Until != nil {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, formatTaskTime(*filter.Until))
	}
	if filter.Search != "" {
		clauses = append(clauses, "(instr(lower(message), lower(?)) > 0 OR instr(lower(code), lower(?)) > 0)")
		args = append(args, filter.Search, filter.Search)
	}
	args = append(args, limit+1)
	// Each task contributes its current lifecycle record exactly once. Worker
	// messages and task-linked transitions retain their durable audit records,
	// but do not duplicate the task in the combined product-facing list.
	rows, err := s.db.QueryContext(ctx, `WITH combined AS (
 SELECT 'task:'||id AS id,updated_at AS occurred_at,'task' AS source,
 CASE WHEN status='failed' THEN 'error' ELSE 'info' END AS level,kind AS code,kind AS message,status,id AS task_id,'{}' AS metadata_json FROM tasks
 UNION ALL
 SELECT 'log:'||l.id,l.occurred_at,l.source,l.level,l.code,l.message,'','',l.metadata_json FROM log_entries l
 WHERE NOT EXISTS(SELECT 1 FROM tasks t WHERE t.id=json_extract(l.metadata_json,'$.task_id'))
 UNION ALL
 SELECT 'runtime:'||id,occurred_at,'runtime',CASE WHEN state='failed' THEN 'error' ELSE 'info' END,reason,reason,state,'','{}' FROM runtime_transitions WHERE task_id IS NULL
 ) SELECT id,occurred_at,source,level,code,message,status,task_id,metadata_json FROM combined WHERE `+strings.Join(clauses, " AND ")+` ORDER BY occurred_at DESC,id DESC LIMIT ?`, args...)
	if err != nil {
		return PanelLogPage{}, fmt.Errorf("query panel log: %w", err)
	}
	defer rows.Close()
	result := PanelLogPage{Items: []PanelLog{}}
	for rows.Next() {
		var item PanelLog
		var at, metadata string
		if err := rows.Scan(&item.ID, &at, &item.Source, &item.Level, &item.Code, &item.Message, &item.Status, &item.TaskID, &metadata); err != nil {
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
