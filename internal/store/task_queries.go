package store

import (
	"context"
	"fmt"
	"strings"
)

// TaskListFilter selects an exact lane, status, and/or kind. Empty fields are
// omitted. Results are newest first.
type TaskListFilter struct {
	Lane   TaskLane
	Status TaskStatus
	Kind   string
	Cursor *CreatedAtCursor
	Limit  int
}

// TaskPage contains one stable keyset page. Next is nil when no later page is
// currently present.
type TaskPage struct {
	Items []Task
	Next  *CreatedAtCursor
}

// ListTasks returns a stable keyset page using only fixed SQL fragments. Every
// caller-controlled filter and cursor value is passed as a query parameter.
func (s *Store) ListTasks(ctx context.Context, filter TaskListFilter) (TaskPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return TaskPage{}, err
	}
	if filter.Lane != "" && filter.Lane != TaskLaneRuntime && filter.Lane != TaskLaneMaintenance {
		return TaskPage{}, fmt.Errorf("invalid task lane %q", filter.Lane)
	}
	if filter.Status != "" && !validTaskStatus(filter.Status) {
		return TaskPage{}, fmt.Errorf("invalid task status %q", filter.Status)
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return TaskPage{}, err
	}

	clauses := []string{"1 = 1"}
	args := make([]any, 0, 8)
	if filter.Lane != "" {
		clauses = append(clauses, "lane = ?")
		args = append(args, string(filter.Lane))
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, filter.Kind)
	}
	if filter.Cursor != nil {
		cursorTime := formatTaskTime(filter.Cursor.CreatedAt)
		clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	args = append(args, limit+1)

	query := `SELECT ` + taskColumns + `
        FROM tasks
        WHERE ` + strings.Join(clauses, " AND ") + `
        ORDER BY created_at DESC, id DESC
        LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return TaskPage{}, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	items := make([]Task, 0, limit+1)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return TaskPage{}, fmt.Errorf("scan listed task: %w", err)
		}
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return TaskPage{}, fmt.Errorf("iterate listed tasks: %w", err)
	}

	page := TaskPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func validTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusQueued,
		TaskStatusRunning,
		TaskStatusSucceeded,
		TaskStatusFailed,
		TaskStatusCanceled,
		TaskStatusSuperseded:
		return true
	default:
		return false
	}
}
