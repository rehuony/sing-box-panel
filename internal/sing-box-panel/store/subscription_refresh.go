// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type SubscriptionRefreshSchedule struct {
	SourceID          string
	ExpectedUpdatedAt time.Time
	NextAt            time.Time
}

func prepareSubscriptionRefreshSchedule(input *SubscriptionRefreshSchedule) (*SubscriptionRefreshSchedule, error) {
	if input == nil {
		return nil, nil
	}
	if input.SourceID == "" || input.ExpectedUpdatedAt.IsZero() || input.NextAt.IsZero() {
		return nil, errors.New("subscription refresh schedule is incomplete")
	}
	copy := *input
	return &copy, nil
}
func saveSubscriptionRefreshScheduleTx(ctx context.Context, tx *sql.Tx, sourceID string, schedule *SubscriptionRefreshSchedule) error {
	if schedule == nil {
		_, err := tx.ExecContext(ctx, `DELETE FROM subscription_refresh_schedule WHERE source_id=?`, sourceID)
		return err
	}
	if schedule.SourceID != sourceID {
		return errors.New("subscription refresh source mismatch")
	}
	var updatedAt string
	if err := tx.QueryRowContext(ctx, `SELECT updated_at FROM subscription_sources WHERE id=?`, sourceID).Scan(&updatedAt); err != nil {
		return err
	}
	if updatedAt != formatTime(schedule.ExpectedUpdatedAt) {
		return ErrSubscriptionConflict
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO subscription_refresh_schedule(source_id,expected_updated_at,next_at) VALUES(?,?,?) ON CONFLICT(source_id) DO UPDATE SET expected_updated_at=excluded.expected_updated_at,next_at=excluded.next_at`, sourceID, updatedAt, formatTime(schedule.NextAt))
	return err
}
func (s *Store) ScheduleSubscriptionRefresh(ctx context.Context, schedule SubscriptionRefreshSchedule) error {
	prepared, err := prepareSubscriptionRefreshSchedule(&schedule)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error { return saveSubscriptionRefreshScheduleTx(ctx, tx, schedule.SourceID, prepared) })
}
func (s *Store) DueSubscriptionRefreshes(ctx context.Context, now time.Time) ([]SubscriptionRefreshSchedule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.source_id,r.expected_updated_at,r.next_at FROM subscription_refresh_schedule r JOIN subscription_sources s ON s.id=r.source_id WHERE r.next_at<=? AND s.enabled=1 AND s.source_kind='remote' AND s.updated_at=r.expected_updated_at ORDER BY r.next_at LIMIT 200`, formatTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SubscriptionRefreshSchedule{}
	for rows.Next() {
		var r SubscriptionRefreshSchedule
		var updated, next string
		if err := rows.Scan(&r.SourceID, &updated, &next); err != nil {
			return nil, err
		}
		r.ExpectedUpdatedAt, err = parseTime(updated)
		if err != nil {
			return nil, err
		}
		r.NextAt, err = parseTime(next)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
