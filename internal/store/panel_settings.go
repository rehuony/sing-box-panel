// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrPanelSettingsConflict = errors.New("panel settings changed")

// CommitPanelSettingsFile couples identity changes and the recovery marker in
// one transaction. publish runs before commit while application readers hold
// the selected file lock; its journal resolves ambiguous commit outcomes.
func (s *Store) CommitPanelSettingsFile(ctx context.Context, path, id string, configuration *ConfigurationFileUpdate, publish func() error) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if configuration != nil {
			if _, err := saveConfigurationFileTx(ctx, tx, configuration.ExpectedRevision, configuration.Content, configuration.Revision); err != nil {
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
