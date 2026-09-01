// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

var (
	ErrRuntimeTransitionNotFound = errors.New("runtime transition not found")
	ErrRuntimeTransitionConflict = errors.New("runtime transition dedupe key conflict")
)

type RuntimeTransitionState string

const (
	RuntimeTransitionRunning RuntimeTransitionState = "running"
	RuntimeTransitionStopped RuntimeTransitionState = "stopped"
	RuntimeTransitionFailed  RuntimeTransitionState = "failed"
	RuntimeTransitionUnknown RuntimeTransitionState = "unknown"
)

// RuntimeTransition is one immutable observation of the managed process
// lifecycle. ProcessStartToken remains an internal fencing value and is not
// exposed by the application transport.
type RuntimeTransition struct {
	ID                 int64
	DedupeKey          string
	State              RuntimeTransitionState
	Reason             string
	ActivationBundleID string
	Generation         int64
	TaskID             string
	PID                int
	ProcessStartToken  string
	ProcessStartedAt   *time.Time
	OccurredAt         time.Time
	UncertainSince     *time.Time
}

type RuntimeTransitionInput struct {
	DedupeKey          string
	State              RuntimeTransitionState
	Reason             string
	ActivationBundleID string
	Generation         int64
	TaskID             string
	PID                int
	ProcessStartToken  string
	ProcessStartedAt   *time.Time
	OccurredAt         time.Time
	UncertainSince     *time.Time
}

type RuntimeTransitionCursor struct {
	OccurredAt time.Time
	ID         int64
}

// RuntimeHistoryFilter selects a newest-first page. From is inclusive, To and
// Cursor are exclusive. Reason is an exact durable event code.
type RuntimeHistoryFilter struct {
	From               *time.Time
	To                 *time.Time
	State              RuntimeTransitionState
	Reason             string
	ActivationBundleID string
	Cursor             *RuntimeTransitionCursor
	Limit              int
}

type RuntimeHistoryPage struct {
	Items            []RuntimeTransition
	Next             *RuntimeTransitionCursor
	Preceding        *RuntimeTransition
	HistoryStartedAt time.Time
}

type runtimeTransitionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// AppendRuntimeTransition is idempotent for one stable dedupe key. Reusing a
// key for different event data fails closed instead of silently rewriting
// append-only history.
func (s *Store) AppendRuntimeTransition(
	ctx context.Context,
	input RuntimeTransitionInput,
) (RuntimeTransition, error) {
	prepared, err := prepareRuntimeTransition(input)
	if err != nil {
		return RuntimeTransition{}, err
	}
	var transition RuntimeTransition
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		transition, err = appendRuntimeTransition(ctx, tx, prepared)
		return err
	})
	return transition, err
}

func appendRuntimeTransition(
	ctx context.Context,
	executor runtimeTransitionExecutor,
	input RuntimeTransitionInput,
) (RuntimeTransition, error) {
	_, err := executor.ExecContext(
		ctx,
		`INSERT INTO runtime_transitions(
            dedupe_key, state, reason, activation_bundle_id, generation,
            task_id, pid, process_start_token, process_started_at,
            occurred_at, uncertain_since
         ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(dedupe_key) DO NOTHING`,
		input.DedupeKey,
		string(input.State),
		input.Reason,
		nullIfEmpty(input.ActivationBundleID),
		nullRuntimeInt64(input.Generation),
		nullIfEmpty(input.TaskID),
		nullRuntimeInt(input.PID),
		nullIfEmpty(input.ProcessStartToken),
		nullRuntimeTime(input.ProcessStartedAt),
		formatTaskTime(input.OccurredAt),
		nullRuntimeTime(input.UncertainSince),
	)
	if err != nil {
		return RuntimeTransition{}, fmt.Errorf("append runtime transition: %w", err)
	}
	stored, err := runtimeTransitionByDedupeKey(ctx, executor, input.DedupeKey)
	if err != nil {
		return RuntimeTransition{}, err
	}
	if !sameRuntimeTransition(stored, input) {
		return RuntimeTransition{}, fmt.Errorf("%w: %s", ErrRuntimeTransitionConflict, input.DedupeKey)
	}
	return stored, nil
}

func (s *Store) LatestRuntimeTransition(ctx context.Context) (RuntimeTransition, error) {
	transition, err := scanRuntimeTransition(s.db.QueryRowContext(
		ctx,
		`SELECT `+runtimeTransitionColumns+`
           FROM runtime_transitions
          ORDER BY occurred_at DESC, id DESC
          LIMIT 1`,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeTransition{}, ErrRuntimeTransitionNotFound
	}
	if err != nil {
		return RuntimeTransition{}, fmt.Errorf("read latest runtime transition: %w", err)
	}
	return transition, nil
}

func (s *Store) ListRuntimeTransitions(
	ctx context.Context,
	filter RuntimeHistoryFilter,
) (RuntimeHistoryPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return RuntimeHistoryPage{}, err
	}
	clauses, args, err := runtimeHistoryClauses(filter)
	if err != nil {
		return RuntimeHistoryPage{}, err
	}
	if filter.Cursor != nil {
		cursorTime := formatTaskTime(filter.Cursor.OccurredAt)
		clauses = append(clauses, "(occurred_at < ? OR (occurred_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+runtimeTransitionColumns+`
           FROM runtime_transitions
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY occurred_at DESC, id DESC
          LIMIT ?`,
		args...,
	)
	if err != nil {
		return RuntimeHistoryPage{}, fmt.Errorf("list runtime transitions: %w", err)
	}
	defer rows.Close()

	items := make([]RuntimeTransition, 0, limit+1)
	for rows.Next() {
		transition, err := scanRuntimeTransition(rows)
		if err != nil {
			return RuntimeHistoryPage{}, fmt.Errorf("scan listed runtime transition: %w", err)
		}
		items = append(items, transition)
	}
	if err := rows.Err(); err != nil {
		return RuntimeHistoryPage{}, fmt.Errorf("iterate listed runtime transitions: %w", err)
	}

	historyStartedAt, err := runtimeHistoryStartedAt(ctx, s.db)
	if err != nil {
		return RuntimeHistoryPage{}, err
	}
	page := RuntimeHistoryPage{Items: items, HistoryStartedAt: historyStartedAt}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &RuntimeTransitionCursor{OccurredAt: last.OccurredAt, ID: last.ID}
	}
	if filter.From != nil {
		preceding, err := precedingRuntimeTransition(ctx, s.db, filter)
		if err != nil && !errors.Is(err, ErrRuntimeTransitionNotFound) {
			return RuntimeHistoryPage{}, err
		}
		if err == nil {
			page.Preceding = &preceding
		}
	}
	return page, nil
}

const runtimeTransitionColumns = `id, dedupe_key, state, reason,
    activation_bundle_id, generation, task_id, pid, process_start_token,
    process_started_at, occurred_at, uncertain_since`

type runtimeTransitionScanner interface {
	Scan(...any) error
}

func scanRuntimeTransition(scanner runtimeTransitionScanner) (RuntimeTransition, error) {
	var (
		transition       RuntimeTransition
		bundleID         sql.NullString
		generation       sql.NullInt64
		taskID           sql.NullString
		pid              sql.NullInt64
		processToken     sql.NullString
		processStartedAt sql.NullString
		occurredAt       string
		uncertainSince   sql.NullString
	)
	if err := scanner.Scan(
		&transition.ID,
		&transition.DedupeKey,
		&transition.State,
		&transition.Reason,
		&bundleID,
		&generation,
		&taskID,
		&pid,
		&processToken,
		&processStartedAt,
		&occurredAt,
		&uncertainSince,
	); err != nil {
		return RuntimeTransition{}, err
	}
	transition.ActivationBundleID = valueOrEmpty(bundleID)
	transition.Generation = generation.Int64
	transition.TaskID = valueOrEmpty(taskID)
	transition.PID = int(pid.Int64)
	transition.ProcessStartToken = valueOrEmpty(processToken)
	var err error
	transition.OccurredAt, err = parseTaskTime(occurredAt)
	if err != nil {
		return RuntimeTransition{}, fmt.Errorf("parse occurred_at: %w", err)
	}
	if processStartedAt.Valid {
		parsed, err := parseTaskTime(processStartedAt.String)
		if err != nil {
			return RuntimeTransition{}, fmt.Errorf("parse process_started_at: %w", err)
		}
		transition.ProcessStartedAt = &parsed
	}
	if uncertainSince.Valid {
		parsed, err := parseTaskTime(uncertainSince.String)
		if err != nil {
			return RuntimeTransition{}, fmt.Errorf("parse uncertain_since: %w", err)
		}
		transition.UncertainSince = &parsed
	}
	return transition, nil
}

func runtimeTransitionByDedupeKey(
	ctx context.Context,
	executor runtimeTransitionExecutor,
	dedupeKey string,
) (RuntimeTransition, error) {
	transition, err := scanRuntimeTransition(executor.QueryRowContext(
		ctx,
		`SELECT `+runtimeTransitionColumns+`
           FROM runtime_transitions
          WHERE dedupe_key = ?`,
		dedupeKey,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeTransition{}, ErrRuntimeTransitionNotFound
	}
	if err != nil {
		return RuntimeTransition{}, fmt.Errorf("read runtime transition: %w", err)
	}
	return transition, nil
}

func runtimeHistoryStartedAt(ctx context.Context, q queryRower) (time.Time, error) {
	var raw string
	if err := q.QueryRowContext(
		ctx,
		`SELECT occurred_at
           FROM runtime_transitions
          WHERE reason = 'history_initialized'
          ORDER BY occurred_at ASC, id ASC
          LIMIT 1`,
	).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, errors.New("runtime history initialization marker is missing")
	} else if err != nil {
		return time.Time{}, fmt.Errorf("read runtime history initialization: %w", err)
	}
	startedAt, err := parseTaskTime(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse runtime history initialization: %w", err)
	}
	return startedAt, nil
}

func precedingRuntimeTransition(
	ctx context.Context,
	q queryRower,
	filter RuntimeHistoryFilter,
) (RuntimeTransition, error) {
	withoutRange := filter
	withoutRange.From = nil
	withoutRange.To = nil
	withoutRange.Cursor = nil
	clauses, args, err := runtimeHistoryClauses(withoutRange)
	if err != nil {
		return RuntimeTransition{}, err
	}
	clauses = append(clauses, "occurred_at < ?")
	args = append(args, formatTaskTime(filter.From.UTC()))
	transition, err := scanRuntimeTransition(q.QueryRowContext(
		ctx,
		`SELECT `+runtimeTransitionColumns+`
           FROM runtime_transitions
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY occurred_at DESC, id DESC
          LIMIT 1`,
		args...,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeTransition{}, ErrRuntimeTransitionNotFound
	}
	if err != nil {
		return RuntimeTransition{}, fmt.Errorf("read preceding runtime transition: %w", err)
	}
	return transition, nil
}

func runtimeHistoryClauses(filter RuntimeHistoryFilter) ([]string, []any, error) {
	if filter.State != "" && !validRuntimeTransitionState(filter.State) {
		return nil, nil, fmt.Errorf("invalid runtime transition state %q", filter.State)
	}
	if filter.Reason != "" && !validRuntimeTransitionReason(filter.Reason) {
		return nil, nil, fmt.Errorf("invalid runtime transition reason %q", filter.Reason)
	}
	if filter.ActivationBundleID != "" && !validRuntimeTransitionIdentifier(filter.ActivationBundleID) {
		return nil, nil, errors.New("invalid runtime transition bundle id")
	}
	if filter.From != nil && filter.From.IsZero() {
		return nil, nil, errors.New("runtime history from time is zero")
	}
	if filter.To != nil && filter.To.IsZero() {
		return nil, nil, errors.New("runtime history to time is zero")
	}
	if filter.From != nil && filter.To != nil && !filter.To.After(*filter.From) {
		return nil, nil, errors.New("runtime history to must be later than from")
	}
	if filter.Cursor != nil && (filter.Cursor.OccurredAt.IsZero() || filter.Cursor.ID < 1) {
		return nil, nil, errors.New("runtime history cursor is invalid")
	}

	clauses := []string{"1 = 1"}
	args := make([]any, 0, 12)
	if filter.From != nil {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, formatTaskTime(filter.From.UTC()))
	}
	if filter.To != nil {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, formatTaskTime(filter.To.UTC()))
	}
	if filter.State != "" {
		clauses = append(clauses, "state = ?")
		args = append(args, string(filter.State))
	}
	if filter.Reason != "" {
		clauses = append(clauses, "reason = ?")
		args = append(args, filter.Reason)
	}
	if filter.ActivationBundleID != "" {
		clauses = append(clauses, "activation_bundle_id = ?")
		args = append(args, filter.ActivationBundleID)
	}
	return clauses, args, nil
}

func prepareRuntimeTransition(input RuntimeTransitionInput) (RuntimeTransitionInput, error) {
	if !validRuntimeTransitionIdentifier(input.DedupeKey) {
		return RuntimeTransitionInput{}, errors.New("runtime transition dedupe key is invalid")
	}
	if !validRuntimeTransitionState(input.State) {
		return RuntimeTransitionInput{}, fmt.Errorf("invalid runtime transition state %q", input.State)
	}
	if !validRuntimeTransitionReason(input.Reason) {
		return RuntimeTransitionInput{}, fmt.Errorf("invalid runtime transition reason %q", input.Reason)
	}
	if input.ActivationBundleID != "" && !validRuntimeTransitionIdentifier(input.ActivationBundleID) {
		return RuntimeTransitionInput{}, errors.New("runtime transition bundle id is invalid")
	}
	if input.TaskID != "" && !validRuntimeTransitionIdentifier(input.TaskID) {
		return RuntimeTransitionInput{}, errors.New("runtime transition task id is invalid")
	}
	if input.Generation < 0 {
		return RuntimeTransitionInput{}, errors.New("runtime transition generation is negative")
	}
	if input.OccurredAt.IsZero() {
		return RuntimeTransitionInput{}, errors.New("runtime transition occurred_at is required")
	}
	if input.PID == 0 {
		if input.ProcessStartToken != "" || input.ProcessStartedAt != nil {
			return RuntimeTransitionInput{}, errors.New("runtime transition process identity is incomplete")
		}
	} else if input.PID < 0 || !validProcessStartToken(input.ProcessStartToken) || input.ProcessStartedAt == nil || input.ProcessStartedAt.IsZero() {
		return RuntimeTransitionInput{}, errors.New("runtime transition process identity is invalid")
	}
	if input.State == RuntimeTransitionRunning && (input.ActivationBundleID == "" || input.PID == 0) {
		return RuntimeTransitionInput{}, errors.New("running transition requires a bundle and exact process identity")
	}
	if input.State == RuntimeTransitionUnknown && input.UncertainSince == nil {
		return RuntimeTransitionInput{}, errors.New("unknown transition requires uncertain_since")
	}
	input.OccurredAt = input.OccurredAt.UTC()
	input.ProcessStartedAt = normalizedRuntimeTime(input.ProcessStartedAt)
	input.UncertainSince = normalizedRuntimeTime(input.UncertainSince)
	if input.ProcessStartedAt != nil && input.ProcessStartedAt.After(input.OccurredAt) {
		return RuntimeTransitionInput{}, errors.New("runtime process start is later than transition")
	}
	if input.UncertainSince != nil && input.UncertainSince.After(input.OccurredAt) {
		return RuntimeTransitionInput{}, errors.New("runtime uncertainty starts after transition")
	}
	return input, nil
}

func validRuntimeTransitionState(state RuntimeTransitionState) bool {
	switch state {
	case RuntimeTransitionRunning, RuntimeTransitionStopped, RuntimeTransitionFailed, RuntimeTransitionUnknown:
		return true
	default:
		return false
	}
}

func validRuntimeTransitionReason(reason string) bool {
	if reason == "" || len(reason) > 128 || strings.TrimSpace(reason) != reason {
		return false
	}
	for index, character := range reason {
		if index == 0 && (character < 'a' || character > 'z') {
			return false
		}
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validRuntimeTransitionIdentifier(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func sameRuntimeTransition(stored RuntimeTransition, input RuntimeTransitionInput) bool {
	return stored.DedupeKey == input.DedupeKey &&
		stored.State == input.State &&
		stored.Reason == input.Reason &&
		stored.ActivationBundleID == input.ActivationBundleID &&
		stored.Generation == input.Generation &&
		stored.TaskID == input.TaskID &&
		stored.PID == input.PID &&
		stored.ProcessStartToken == input.ProcessStartToken &&
		sameRuntimeTime(stored.ProcessStartedAt, input.ProcessStartedAt) &&
		stored.OccurredAt.Equal(input.OccurredAt) &&
		sameRuntimeTime(stored.UncertainSince, input.UncertainSince)
}

func normalizedRuntimeTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func sameRuntimeTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func nullRuntimeTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTaskTime(value.UTC())
}

func nullRuntimeInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullRuntimeInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
