// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrPanelSettingsConflict = errors.New("panel settings changed")

// PanelSettings reads legacy preferences for one-time migration into the file.
func (s *Store) PanelSettings(ctx context.Context) (json.RawMessage, int64, error) {
	var document string
	var revision int64
	err := s.db.QueryRowContext(ctx, "SELECT document, revision FROM panel_settings WHERE singleton = 1").Scan(&document, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read panel settings: %w", err)
	}
	return json.RawMessage(document), revision, nil
}

// CommitPanelSettingsFile couples identity changes and the recovery marker in
// one transaction. publish runs before commit while application readers hold
// the selected file lock; its journal resolves ambiguous commit outcomes.
func (s *Store) CommitPanelSettingsFile(ctx context.Context, path, id string, legacyRevision *int64, configuration *ConfigurationFileUpdate, publish func() error) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if legacyRevision != nil {
			var current int64
			err := tx.QueryRowContext(ctx, "SELECT revision FROM panel_settings WHERE singleton=1").Scan(&current)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if current != *legacyRevision {
				return ErrPanelSettingsConflict
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM panel_settings WHERE singleton=1"); err != nil {
				return err
			}
		}
		if configuration != nil {
			if _, err := saveConfigurationFileTx(ctx, tx, configuration.ExpectedRevision, configuration.Content, configuration.Revision, configuration.Task); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO panel_settings_file_commits(settings_path, transaction_id) VALUES(?,?)
    ON CONFLICT(settings_path) DO UPDATE SET transaction_id=excluded.transaction_id`, path, id); err != nil {
			return err
		}
		return publish()
	})
}

// PanelSettingsFileCommitted uses transaction identity so alternate spellings or
// symlinked parent directories of the same settings file recover identically.
func (s *Store) PanelSettingsFileCommitted(ctx context.Context, id string) (bool, error) {
	var committed bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM panel_settings_file_commits WHERE transaction_id=?)", id).Scan(&committed)
	return committed, err
}
