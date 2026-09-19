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

// PanelSettings returns revision zero before the first save. Bootstrap defaults
// are resolved by the application, not frozen into a database migration.
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

// SavePanelSettings uses a compare-and-swap even for the first save. No settings
// values are included in errors because the document contains credentials.
func (s *Store) SavePanelSettings(ctx context.Context, document json.RawMessage, expected int64, configuration *ConfigurationFileUpdate) (int64, error) {
	if expected < 0 || len(document) > 64<<10 || !json.Valid(document) {
		return 0, errors.New("invalid panel settings document")
	}
	var revision int64
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var current int64
		err := tx.QueryRowContext(ctx, "SELECT revision FROM panel_settings WHERE singleton = 1").Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read settings revision: %w", err)
		}
		if current != expected {
			return ErrPanelSettingsConflict
		}
		if configuration != nil {
			if _, err := saveConfigurationFileTx(ctx, tx, configuration.ExpectedRevision, configuration.Content, configuration.Revision, configuration.Task); err != nil {
				return err
			}
		}
		revision = current + 1
		_, err = tx.ExecContext(ctx, `INSERT INTO panel_settings(singleton, revision, document) VALUES(1, ?, ?)
			ON CONFLICT(singleton) DO UPDATE SET revision=excluded.revision, document=excluded.document`, revision, string(document))
		if err != nil {
			return errors.New("save panel settings failed")
		}
		return nil
	})
	return revision, err
}
