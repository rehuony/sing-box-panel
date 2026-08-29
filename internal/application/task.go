// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

type Task struct {
	ID                  string           `json:"id"`
	IdempotencyKey      string           `json:"idempotency_key,omitempty"`
	Lane                store.TaskLane   `json:"lane"`
	Kind                store.TaskKind   `json:"kind"`
	Status              store.TaskStatus `json:"status"`
	Generation          int64            `json:"generation"`
	CanonicalRevisionID string           `json:"canonical_revision_id,omitempty"`
	StartupArtifactID   string           `json:"startup_artifact_id,omitempty"`
	ActivationBundleID  string           `json:"activation_bundle_id,omitempty"`
	Payload             json.RawMessage  `json:"payload"`
	Result              json.RawMessage  `json:"result,omitempty"`
	Failure             json.RawMessage  `json:"failure,omitempty"`
	CancelRequested     bool             `json:"cancel_requested"`
	Attempt             int              `json:"attempt"`
	LeaseExpiresAt      *time.Time       `json:"lease_expires_at,omitempty"`
	NotBefore           *time.Time       `json:"not_before,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

func (task Task) Terminal() bool {
	switch task.Status {
	case store.TaskStatusSucceeded, store.TaskStatusFailed, store.TaskStatusCanceled, store.TaskStatusSuperseded:
		return true
	default:
		return false
	}
}

type TaskListFilter struct {
	Lane   store.TaskLane
	Status store.TaskStatus
	Kind   store.TaskKind
	Cursor *store.CreatedAtCursor
	Limit  int
}

type TaskPage struct {
	Items []Task      `json:"items"`
	Next  *TaskCursor `json:"next,omitempty"`
}

type TaskCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func (application *Application) ListTasks(ctx context.Context, filter TaskListFilter) (TaskPage, error) {
	page, err := application.database.ListTasks(ctx, store.TaskListFilter{
		Lane:   filter.Lane,
		Status: filter.Status,
		Kind:   filter.Kind,
		Cursor: filter.Cursor,
		Limit:  filter.Limit,
	})
	if err != nil {
		return TaskPage{}, err
	}
	result := TaskPage{Items: make([]Task, len(page.Items))}
	for index, task := range page.Items {
		result.Items[index] = applicationTask(task)
	}
	if page.Next != nil {
		result.Next = &TaskCursor{CreatedAt: page.Next.CreatedAt, ID: page.Next.ID}
	}
	return result, nil
}

func (application *Application) Task(ctx context.Context, taskID string) (Task, error) {
	task, err := application.database.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	return applicationTask(task), nil
}

func (application *Application) CancelTask(ctx context.Context, taskID string) (Task, error) {
	task, err := application.database.RequestTaskCancellation(ctx, taskID, application.now().UTC())
	if err != nil {
		return Task{}, err
	}
	return applicationTask(task), nil
}
func applicationTask(value store.Task) Task {
	return Task{
		ID:                  value.ID,
		IdempotencyKey:      value.IdempotencyKey,
		Lane:                value.Lane,
		Kind:                value.Kind,
		Status:              value.Status,
		Generation:          value.Generation,
		CanonicalRevisionID: value.CanonicalRevisionID,
		StartupArtifactID:   value.StartupArtifactID,
		ActivationBundleID:  value.ActivationBundleID,
		Payload:             append(json.RawMessage(nil), value.Payload...),
		Result:              append(json.RawMessage(nil), value.Result...),
		Failure:             append(json.RawMessage(nil), value.Failure...),
		CancelRequested:     value.CancelRequested,
		Attempt:             value.Attempt,
		LeaseExpiresAt:      cloneTime(value.LeaseExpiresAt),
		NotBefore:           cloneTime(value.NotBefore),
		CreatedAt:           value.CreatedAt,
		UpdatedAt:           value.UpdatedAt,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
