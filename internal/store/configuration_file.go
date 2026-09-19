// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

var (
	ErrConfigurationFileConflict = errors.New("configuration file changed")
	ErrConfigurationFileInvalid  = errors.New("configuration file is not valid UTF-8 text within the size limit")
	ErrConfigurationFileUnparsed = errors.New("saved configuration must be corrected before using structured or runtime operations")
)

// ConfigurationFile is the one editable file, separate from checked runtime
// bytes. An empty canonical ID means this saved text cannot be compiled.
type ConfigurationFile struct {
	Revision            int64
	Content             string
	CanonicalRevisionID string
	UpdatedAt           time.Time
}

func readConfigurationFile(row interface{ Scan(...any) error }) (ConfigurationFile, error) {
	var value ConfigurationFile
	var canonical sql.NullString
	var timestamp string
	err := row.Scan(&value.Revision, &value.Content, &canonical, &timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigurationFile{Content: "{}"}, nil
	}
	if err != nil {
		return ConfigurationFile{}, fmt.Errorf("read configuration file: %w", err)
	}
	value.CanonicalRevisionID = valueOrEmpty(canonical)
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, timestamp)
	return value, err
}

const configurationFileColumns = `revision, content, canonical_revision_id, updated_at`

func (s *Store) ConfigurationFile(ctx context.Context) (ConfigurationFile, error) {
	return readConfigurationFile(s.db.QueryRowContext(ctx, `SELECT `+configurationFileColumns+` FROM configuration_file WHERE singleton=1`))
}

// SaveConfigurationFile preserves exact text, even when JSON is incomplete.
// Valid documents advance the immutable canonical head and task in the same
// transaction. Invalid documents never substitute an old valid configuration.
func (s *Store) SaveConfigurationFile(ctx context.Context, expected int64, content string, revision NewCanonicalRevision, task NewTask) (ConfigurationFile, error) {
	var result ConfigurationFile
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = saveConfigurationFileTx(ctx, tx, expected, content, revision, task)
		return err
	})
	return result, err
}

// ConfigurationFileUpdate joins a settings change to the same saved-file CAS.
type ConfigurationFileUpdate struct {
	ExpectedRevision int64
	Content          string
	Revision         NewCanonicalRevision
	Task             NewTask
}

func saveConfigurationFileTx(ctx context.Context, tx *sql.Tx, expected int64, content string, revision NewCanonicalRevision, task NewTask) (ConfigurationFile, error) {
	if expected < 0 || len(content) > configuration.MaximumBytes || !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return ConfigurationFile{}, ErrConfigurationFileInvalid
	}
	document, parseErr := configuration.Parse([]byte(content))
	var prepared CanonicalRevision
	var preparedTask NewTask
	var err error
	if parseErr == nil {
		revision.Document = document.CanonicalJSON()
		prepared, preparedTask, err = prepareCanonicalSave(revision, task)
		if err != nil {
			return ConfigurationFile{}, err
		}
	}
	if revision.CreatedAt.IsZero() {
		revision.CreatedAt = time.Now().UTC()
	}
	var result ConfigurationFile
	err = func() error {
		current, err := readConfigurationFile(tx.QueryRowContext(ctx, `SELECT `+configurationFileColumns+` FROM configuration_file WHERE singleton=1`))
		if err != nil {
			return err
		}
		if current.Revision != expected {
			return ErrConfigurationFileConflict
		}
		if current.Revision > 0 && current.Content == content {
			result = current
			return nil
		}
		var canonicalID string
		if parseErr == nil {
			var head sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT head_revision_id FROM hub_state WHERE singleton=1`).Scan(&head); err != nil {
				return err
			}
			stored, inserted, err := saveCanonicalRevisionTx(ctx, tx, valueOrEmpty(head), prepared, true)
			if err != nil {
				return err
			}
			canonicalID = stored.ID
			if inserted {
				if err := insertCanonicalTaskTx(ctx, tx, preparedTask, stored.ID, ""); err != nil {
					return err
				}
			}
		}
		result = ConfigurationFile{Revision: current.Revision + 1, Content: content, CanonicalRevisionID: canonicalID, UpdatedAt: revision.CreatedAt.UTC()}
		return writeConfigurationFileTx(ctx, tx, result)
	}()
	return result, err
}

func writeConfigurationFileTx(ctx context.Context, tx *sql.Tx, value ConfigurationFile) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO configuration_file(singleton, revision, content, canonical_revision_id, updated_at) VALUES(1, ?, ?, ?, ?)
        ON CONFLICT(singleton) DO UPDATE SET revision=excluded.revision, content=excluded.content, canonical_revision_id=excluded.canonical_revision_id, updated_at=excluded.updated_at`,
		value.Revision, value.Content, nullIfEmpty(value.CanonicalRevisionID), value.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save configuration file: %w", err)
	}
	return nil
}

// Legacy JSON-pointer/full-document writes remain supported, but cannot
// silently overwrite invalid saved text that they cannot represent.
func syncCanonicalFileTx(ctx context.Context, tx *sql.Tx, revision CanonicalRevision) error {
	current, err := readConfigurationFile(tx.QueryRowContext(ctx, `SELECT `+configurationFileColumns+` FROM configuration_file WHERE singleton=1`))
	if err != nil {
		return err
	}
	if current.Revision > 0 && current.CanonicalRevisionID == "" {
		return ErrConfigurationFileUnparsed
	}
	return writeConfigurationFileTx(ctx, tx, ConfigurationFile{
		Revision: current.Revision + 1, Content: string(revision.Document), CanonicalRevisionID: revision.ID, UpdatedAt: revision.CreatedAt,
	})
}

func requireCurrentConfigurationFileTx(ctx context.Context, tx *sql.Tx, canonicalID string) error {
	current, err := readConfigurationFile(tx.QueryRowContext(ctx, `SELECT `+configurationFileColumns+` FROM configuration_file WHERE singleton=1`))
	if err != nil {
		return err
	}
	if current.Revision > 0 && current.CanonicalRevisionID != canonicalID {
		return ErrCompiledStartupEvidenceStale
	}
	return nil
}
