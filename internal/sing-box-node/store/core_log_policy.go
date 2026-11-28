// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CoreLogPolicy is the core log retention the panel last sent.
type CoreLogPolicy struct {
	RetentionDays int
	MaxFiles      int
	MaxFileBytes  int64
}

// CoreLogPolicy returns the policy the panel last sent, or false before it
// sent one.
func (s *Store) CoreLogPolicy(ctx context.Context) (CoreLogPolicy, bool, error) {
	var policy CoreLogPolicy
	err := s.db.QueryRowContext(ctx, `SELECT retention_days, max_files, max_file_bytes
           FROM core_log_policy WHERE singleton = 1`).Scan(&policy.RetentionDays, &policy.MaxFiles, &policy.MaxFileBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return CoreLogPolicy{}, false, nil
	}
	if err != nil {
		return CoreLogPolicy{}, false, fmt.Errorf("read core log policy: %w", err)
	}
	return policy, true, nil
}

// SaveCoreLogPolicy replaces the policy the panel last sent.
func (s *Store) SaveCoreLogPolicy(ctx context.Context, policy CoreLogPolicy, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO core_log_policy(singleton, retention_days, max_files, max_file_bytes, updated_at)
         VALUES (1, ?, ?, ?, ?)
         ON CONFLICT(singleton) DO UPDATE SET
           retention_days = excluded.retention_days, max_files = excluded.max_files,
           max_file_bytes = excluded.max_file_bytes, updated_at = excluded.updated_at`,
		policy.RetentionDays, policy.MaxFiles, policy.MaxFileBytes, formatTime(at)); err != nil {
		return fmt.Errorf("save core log policy: %w", err)
	}
	return nil
}
