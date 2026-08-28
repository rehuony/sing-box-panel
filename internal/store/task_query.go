// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) GetTask(ctx context.Context, taskID string) (Task, error) {
	if strings.TrimSpace(taskID) == "" {
		return Task{}, errors.New("task id is empty")
	}
	return getTask(ctx, s.db, taskID)
}

func getTask(ctx context.Context, q queryRower, taskID string) (Task, error) {
	row := q.QueryRowContext(
		ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE id = ?`,
		taskID,
	)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if err != nil {
		return Task{}, fmt.Errorf("scan task %q: %w", taskID, err)
	}
	return task, nil
}

func scanTask(row taskScanner) (Task, error) {
	var (
		task                Task
		idempotencyKey      sql.NullString
		canonicalRevisionID sql.NullString
		startupArtifactID   sql.NullString
		activationBundleID  sql.NullString
		payload             string
		result              sql.NullString
		failure             sql.NullString
		cancelRequested     int
		leaseOwner          sql.NullString
		leaseExpiresAt      sql.NullString
		notBefore           sql.NullString
		createdAt           string
		updatedAt           string
	)
	if err := row.Scan(
		&task.ID,
		&idempotencyKey,
		&task.Lane,
		&task.Kind,
		&task.Status,
		&task.Generation,
		&canonicalRevisionID,
		&startupArtifactID,
		&activationBundleID,
		&payload,
		&result,
		&failure,
		&cancelRequested,
		&task.Attempt,
		&leaseOwner,
		&leaseExpiresAt,
		&notBefore,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Task{}, err
	}

	var err error
	task.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse created_at: %w", err)
	}
	task.UpdatedAt, err = parseTaskTime(updatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if leaseExpiresAt.Valid {
		parsed, err := parseTaskTime(leaseExpiresAt.String)
		if err != nil {
			return Task{}, fmt.Errorf("parse lease_expires_at: %w", err)
		}
		task.LeaseExpiresAt = &parsed
	}
	if notBefore.Valid {
		parsed, err := parseTaskTime(notBefore.String)
		if err != nil {
			return Task{}, fmt.Errorf("parse not_before: %w", err)
		}
		task.NotBefore = &parsed
	}

	task.IdempotencyKey = valueOrEmpty(idempotencyKey)
	task.CanonicalRevisionID = valueOrEmpty(canonicalRevisionID)
	task.StartupArtifactID = valueOrEmpty(startupArtifactID)
	task.ActivationBundleID = valueOrEmpty(activationBundleID)
	task.Payload = json.RawMessage(payload)
	if result.Valid {
		task.Result = json.RawMessage(result.String)
	}
	if failure.Valid {
		task.Failure = json.RawMessage(failure.String)
	}
	task.CancelRequested = cancelRequested != 0
	task.LeaseOwner = valueOrEmpty(leaseOwner)
	return task, nil
}
