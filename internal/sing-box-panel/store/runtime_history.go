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
	ErrRuntimeTransitionConflict = errors.New("runtime transition conflicts with the recorded entry")
)

type RuntimeTransitionState string

const (
	RuntimeTransitionRunning RuntimeTransitionState = "running"
	RuntimeTransitionStopped RuntimeTransitionState = "stopped"
	RuntimeTransitionFailed  RuntimeTransitionState = "failed"
	RuntimeTransitionUnknown RuntimeTransitionState = "unknown"
)

// RuntimeTransition is one immutable entry of a node's runtime history.
type RuntimeTransition struct {
	ID                 int64
	NodeID             string
	State              RuntimeTransitionState
	Reason             string
	ActivationBundleID string
	Generation         int64
	PID                int
	ProcessStartedAt   *time.Time
	OccurredAt         time.Time
	UncertainSince     *time.Time
}

// RuntimeTransitionInput is one entry of a node instance's history, identified
// by its ID there.
type RuntimeTransitionInput struct {
	NodeID             string
	InstanceID         string
	SourceID           int64
	State              RuntimeTransitionState
	Reason             string
	ActivationBundleID string
	Generation         int64
	PID                int
	ProcessStartedAt   *time.Time
	OccurredAt         time.Time
	UncertainSince     *time.Time
}

type RuntimeTransitionCursor struct {
	OccurredAt time.Time
	ID         int64
}

// RuntimeHistoryFilter selects a newest-first page of one node's history.
// From is inclusive, To and Cursor are exclusive. Reason is an exact durable
// event code.
type RuntimeHistoryFilter struct {
	NodeID             string
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

// RecordRuntimeTransition stores a replicated history entry. Recording the
// same entry again does nothing; an entry whose source ID was recorded with
// other data fails closed instead of rewriting history.
func (s *Store) RecordRuntimeTransition(ctx context.Context, input RuntimeTransitionInput) error {
	prepared, err := prepareRuntimeTransition(input)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		inserted, err := tx.ExecContext(ctx, `INSERT INTO runtime_transitions(
            node_id, instance_id, source_id, state, reason, activation_bundle_id, generation,
            pid, process_started_at, occurred_at, uncertain_since
         ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(node_id, instance_id, source_id) DO NOTHING`,
			prepared.NodeID, prepared.InstanceID, prepared.SourceID, string(prepared.State), prepared.Reason,
			nullIfEmpty(prepared.ActivationBundleID), nullRuntimeInt64(prepared.Generation), nullRuntimeInt(prepared.PID),
			nullRuntimeTime(prepared.ProcessStartedAt), formatTime(prepared.OccurredAt), nullRuntimeTime(prepared.UncertainSince))
		if err != nil {
			return fmt.Errorf("record runtime transition: %w", err)
		}
		if count, err := inserted.RowsAffected(); err != nil || count == 1 {
			return err
		}
		stored, err := scanRuntimeTransition(tx.QueryRowContext(ctx, `SELECT `+runtimeTransitionColumns+`
           FROM runtime_transitions WHERE node_id = ? AND instance_id = ? AND source_id = ?`,
			prepared.NodeID, prepared.InstanceID, prepared.SourceID))
		if err != nil {
			return fmt.Errorf("read recorded runtime transition: %w", err)
		}
		if !sameRuntimeTransition(stored, prepared) {
			return fmt.Errorf("%w: %s/%d", ErrRuntimeTransitionConflict, prepared.InstanceID, prepared.SourceID)
		}
		return nil
	})
}

// LatestRuntimeTransitionSource returns the newest source ID recorded for the
// node instance, or zero.
func (s *Store) LatestRuntimeTransitionSource(ctx context.Context, nodeID, instanceID string) (int64, error) {
	var latest sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(source_id) FROM runtime_transitions WHERE node_id = ? AND instance_id = ?`,
		nodeID, instanceID).Scan(&latest); err != nil {
		return 0, fmt.Errorf("read latest runtime transition: %w", err)
	}
	return latest.Int64, nil
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
		cursorTime := formatTime(filter.Cursor.OccurredAt)
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

	historyStartedAt, err := runtimeHistoryStartedAt(ctx, s.db, filter.NodeID)
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

const runtimeTransitionColumns = `id, node_id, state, reason,
    activation_bundle_id, generation, pid,
    process_started_at, occurred_at, uncertain_since`

type runtimeTransitionScanner interface {
	Scan(...any) error
}

func scanRuntimeTransition(scanner runtimeTransitionScanner) (RuntimeTransition, error) {
	var (
		transition       RuntimeTransition
		bundleID         sql.NullString
		generation       sql.NullInt64
		pid              sql.NullInt64
		processStartedAt sql.NullString
		occurredAt       string
		uncertainSince   sql.NullString
	)
	if err := scanner.Scan(
		&transition.ID,
		&transition.NodeID,
		&transition.State,
		&transition.Reason,
		&bundleID,
		&generation,
		&pid,
		&processStartedAt,
		&occurredAt,
		&uncertainSince,
	); err != nil {
		return RuntimeTransition{}, err
	}
	transition.ActivationBundleID = valueOrEmpty(bundleID)
	transition.Generation = generation.Int64
	transition.PID = int(pid.Int64)
	var err error
	transition.OccurredAt, err = parseTime(occurredAt)
	if err != nil {
		return RuntimeTransition{}, fmt.Errorf("parse occurred_at: %w", err)
	}
	if processStartedAt.Valid {
		parsed, err := parseTime(processStartedAt.String)
		if err != nil {
			return RuntimeTransition{}, fmt.Errorf("parse process_started_at: %w", err)
		}
		transition.ProcessStartedAt = &parsed
	}
	if uncertainSince.Valid {
		parsed, err := parseTime(uncertainSince.String)
		if err != nil {
			return RuntimeTransition{}, fmt.Errorf("parse uncertain_since: %w", err)
		}
		transition.UncertainSince = &parsed
	}
	return transition, nil
}

// runtimeHistoryStartedAt is when the node's history began: the first
// initialization marker it reported, or the node's creation before it
// reported any history.
func runtimeHistoryStartedAt(ctx context.Context, q queryRower, nodeID string) (time.Time, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT occurred_at FROM runtime_transitions
          WHERE node_id = ? AND reason = 'history_initialized'
          ORDER BY occurred_at ASC, id ASC LIMIT 1`, nodeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		err = q.QueryRowContext(ctx, `SELECT created_at FROM nodes WHERE id = ?`, nodeID).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrNodeNotFound
		}
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read runtime history start: %w", err)
	}
	startedAt, err := parseTime(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse runtime history start: %w", err)
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
	args = append(args, formatTime(filter.From.UTC()))
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
	if !ValidIdentifier(filter.NodeID) {
		return nil, nil, errors.New("runtime history requires a node")
	}
	if filter.State != "" && !validRuntimeTransitionState(filter.State) {
		return nil, nil, fmt.Errorf("invalid runtime transition state %q", filter.State)
	}
	if filter.Reason != "" && !validRuntimeTransitionReason(filter.Reason) {
		return nil, nil, fmt.Errorf("invalid runtime transition reason %q", filter.Reason)
	}
	if filter.ActivationBundleID != "" && !ValidIdentifier(filter.ActivationBundleID) {
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

	clauses := []string{"node_id = ?"}
	args := []any{filter.NodeID}
	if filter.From != nil {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, formatTime(filter.From.UTC()))
	}
	if filter.To != nil {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, formatTime(filter.To.UTC()))
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
	if !ValidIdentifier(input.NodeID) || input.InstanceID == "" || len(input.InstanceID) > 64 || input.SourceID < 1 {
		return RuntimeTransitionInput{}, errors.New("runtime transition source is invalid")
	}
	if !validRuntimeTransitionState(input.State) {
		return RuntimeTransitionInput{}, fmt.Errorf("invalid runtime transition state %q", input.State)
	}
	if !validRuntimeTransitionReason(input.Reason) {
		return RuntimeTransitionInput{}, fmt.Errorf("invalid runtime transition reason %q", input.Reason)
	}
	if input.ActivationBundleID != "" && !ValidIdentifier(input.ActivationBundleID) {
		return RuntimeTransitionInput{}, errors.New("runtime transition bundle id is invalid")
	}
	if input.Generation < 0 {
		return RuntimeTransitionInput{}, errors.New("runtime transition generation is negative")
	}
	if input.OccurredAt.IsZero() {
		return RuntimeTransitionInput{}, errors.New("runtime transition occurred_at is required")
	}
	if input.PID < 0 || (input.PID == 0) != (input.ProcessStartedAt == nil) ||
		(input.ProcessStartedAt != nil && input.ProcessStartedAt.IsZero()) {
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

// ValidIdentifier reports whether value is a stable record identifier: 1 to 256
// bytes with no surrounding space and no control characters.
func ValidIdentifier(value string) bool {
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
	return stored.State == input.State &&
		stored.Reason == input.Reason &&
		stored.ActivationBundleID == input.ActivationBundleID &&
		stored.Generation == input.Generation &&
		stored.PID == input.PID &&
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
	return formatTime(value.UTC())
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
