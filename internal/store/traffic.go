// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrTrafficPeriodNotFound = errors.New("traffic period not found")

type TrafficPeriod struct {
	ID                 string          `json:"id"`
	ActivationBundleID string          `json:"activation_bundle_id,omitempty"`
	PeriodStart        time.Time       `json:"period_start"`
	PeriodEnd          time.Time       `json:"period_end"`
	InboundBytes       int64           `json:"inbound_bytes"`
	OutboundBytes      int64           `json:"outbound_bytes"`
	Counters           json.RawMessage `json:"counters"`
	CreatedAt          time.Time       `json:"created_at"`
}

type TrafficPeriodFilter struct {
	ActivationBundleID string
	OverlapsStart      *time.Time
	OverlapsEnd        *time.Time
	Limit              int
}

// UpsertTrafficPeriod records an actual collector sample. Identity and period
// boundaries are immutable, and byte totals may only increase.
func (s *Store) UpsertTrafficPeriod(ctx context.Context, period TrafficPeriod) (TrafficPeriod, error) {
	prepared, err := prepareTrafficPeriod(period)
	if err != nil {
		return TrafficPeriod{}, err
	}
	var stored TrafficPeriod
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, getErr := getTrafficPeriod(ctx, tx, prepared.ID)
		switch {
		case getErr == nil:
			if current.ActivationBundleID != prepared.ActivationBundleID ||
				!current.PeriodStart.Equal(prepared.PeriodStart) ||
				!current.PeriodEnd.Equal(prepared.PeriodEnd) {
				return errors.New("traffic period immutable identity does not match")
			}
			if prepared.InboundBytes < current.InboundBytes || prepared.OutboundBytes < current.OutboundBytes {
				return errors.New("traffic counters cannot decrease")
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE traffic_periods
                    SET inbound_bytes = ?, outbound_bytes = ?, counters_json = ?
                  WHERE id = ?`,
				prepared.InboundBytes,
				prepared.OutboundBytes,
				string(prepared.Counters),
				prepared.ID,
			); err != nil {
				return fmt.Errorf("update traffic period: %w", err)
			}
		case errors.Is(getErr, ErrTrafficPeriodNotFound):
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO traffic_periods(
                    id, activation_bundle_id, period_start, period_end,
                    inbound_bytes, outbound_bytes, counters_json, created_at
                 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				prepared.ID,
				nullIfEmpty(prepared.ActivationBundleID),
				formatTaskTime(prepared.PeriodStart),
				formatTaskTime(prepared.PeriodEnd),
				prepared.InboundBytes,
				prepared.OutboundBytes,
				string(prepared.Counters),
				formatTaskTime(prepared.CreatedAt),
			); err != nil {
				return fmt.Errorf("insert traffic period: %w", err)
			}
		default:
			return getErr
		}
		stored, err = getTrafficPeriod(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func (s *Store) GetTrafficPeriod(ctx context.Context, periodID string) (TrafficPeriod, error) {
	if strings.TrimSpace(periodID) == "" {
		return TrafficPeriod{}, errors.New("traffic period id is empty")
	}
	return getTrafficPeriod(ctx, s.db, periodID)
}

func (s *Store) CurrentTrafficPeriod(ctx context.Context, at time.Time) (TrafficPeriod, error) {
	if at.IsZero() {
		return TrafficPeriod{}, errors.New("traffic period lookup time is required")
	}
	period, err := scanTrafficPeriod(s.db.QueryRowContext(
		ctx,
		`SELECT id, activation_bundle_id, period_start, period_end,
                inbound_bytes, outbound_bytes, counters_json, created_at
           FROM traffic_periods
          WHERE period_start <= ? AND period_end > ?
          ORDER BY period_start DESC, id DESC LIMIT 1`,
		formatTaskTime(at.UTC()),
		formatTaskTime(at.UTC()),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficPeriod{}, ErrTrafficPeriodNotFound
	}
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("get current traffic period: %w", err)
	}
	return period, nil
}

func (s *Store) ListTrafficPeriods(ctx context.Context, filter TrafficPeriodFilter) ([]TrafficPeriod, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return nil, err
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 6)
	if filter.ActivationBundleID != "" {
		clauses = append(clauses, "activation_bundle_id = ?")
		args = append(args, filter.ActivationBundleID)
	}
	if filter.OverlapsStart != nil {
		clauses = append(clauses, "period_end > ?")
		args = append(args, formatTaskTime(filter.OverlapsStart.UTC()))
	}
	if filter.OverlapsEnd != nil {
		clauses = append(clauses, "period_start < ?")
		args = append(args, formatTaskTime(filter.OverlapsEnd.UTC()))
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, activation_bundle_id, period_start, period_end,
                inbound_bytes, outbound_bytes, counters_json, created_at
           FROM traffic_periods
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY period_start DESC, id DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list traffic periods: %w", err)
	}
	defer rows.Close()
	periods := make([]TrafficPeriod, 0)
	for rows.Next() {
		period, err := scanTrafficPeriod(rows)
		if err != nil {
			return nil, fmt.Errorf("scan traffic period: %w", err)
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic periods: %w", err)
	}
	return periods, nil
}

func prepareTrafficPeriod(period TrafficPeriod) (TrafficPeriod, error) {
	if strings.TrimSpace(period.ID) == "" || strings.TrimSpace(period.ID) != period.ID {
		return TrafficPeriod{}, errors.New("traffic period id is invalid")
	}
	if period.PeriodStart.IsZero() || period.PeriodEnd.IsZero() || !period.PeriodEnd.After(period.PeriodStart) {
		return TrafficPeriod{}, errors.New("traffic period boundaries are invalid")
	}
	if period.InboundBytes < 0 || period.OutboundBytes < 0 {
		return TrafficPeriod{}, errors.New("traffic byte counters cannot be negative")
	}
	counters, err := canonicalJSONObject(period.Counters, `{}`)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("traffic counters: %w", err)
	}
	period.PeriodStart = period.PeriodStart.UTC()
	period.PeriodEnd = period.PeriodEnd.UTC()
	period.Counters = counters
	if period.CreatedAt.IsZero() {
		period.CreatedAt = time.Now().UTC()
	} else {
		period.CreatedAt = period.CreatedAt.UTC()
	}
	return period, nil
}

func getTrafficPeriod(ctx context.Context, q queryRower, periodID string) (TrafficPeriod, error) {
	period, err := scanTrafficPeriod(q.QueryRowContext(
		ctx,
		`SELECT id, activation_bundle_id, period_start, period_end,
                inbound_bytes, outbound_bytes, counters_json, created_at
           FROM traffic_periods WHERE id = ?`,
		periodID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficPeriod{}, fmt.Errorf("%w: %s", ErrTrafficPeriodNotFound, periodID)
	}
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("get traffic period: %w", err)
	}
	return period, nil
}

func scanTrafficPeriod(row taskScanner) (TrafficPeriod, error) {
	var period TrafficPeriod
	var bundleID sql.NullString
	var periodStart, periodEnd, counters, createdAt string
	if err := row.Scan(
		&period.ID,
		&bundleID,
		&periodStart,
		&periodEnd,
		&period.InboundBytes,
		&period.OutboundBytes,
		&counters,
		&createdAt,
	); err != nil {
		return TrafficPeriod{}, err
	}
	var err error
	period.ActivationBundleID = valueOrEmpty(bundleID)
	period.PeriodStart, err = parseTaskTime(periodStart)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("parse period_start: %w", err)
	}
	period.PeriodEnd, err = parseTaskTime(periodEnd)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("parse period_end: %w", err)
	}
	period.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return TrafficPeriod{}, fmt.Errorf("parse created_at: %w", err)
	}
	period.Counters = append(json.RawMessage(nil), counters...)
	return period, nil
}
