// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrStartupArtifactNotFound = errors.New("startup artifact not found")
	ErrStartupArtifactExists   = errors.New("startup artifact already exists")
	ErrStartupArtifactState    = errors.New("startup artifact state transition is invalid")
)

type StartupArtifactState string

const (
	StartupArtifactPending StartupArtifactState = "pending"
	StartupArtifactReady   StartupArtifactState = "ready"
	StartupArtifactFailed  StartupArtifactState = "failed"
)

type StartupArtifact struct {
	ID                  string               `json:"id"`
	CanonicalRevisionID string               `json:"canonical_revision_id"`
	ExactCoreVersion    string               `json:"exact_core_version"`
	CoreArtifactID      string               `json:"core_artifact_id"`
	ConfigBytes         []byte               `json:"-"`
	ConfigSHA256        string               `json:"config_sha256"`
	State               StartupArtifactState `json:"state"`
	CheckedAt           *time.Time           `json:"checked_at,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

const startupArtifactColumns = `
	    id, canonical_revision_id, exact_core_version, core_artifact_id,
	    config_bytes, config_sha256, state, checked_at, created_at`

func insertStartupArtifactTx(
	ctx context.Context,
	tx *sql.Tx,
	prepared StartupArtifact,
) (StartupArtifact, error) {
	if err := validateStartupArtifactReferences(ctx, tx, prepared); err != nil {
		return StartupArtifact{}, err
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO startup_artifacts(
	                id, canonical_revision_id, exact_core_version, core_artifact_id,
	                config_bytes, config_sha256, state, checked_at, created_at
	             ) VALUES (?, ?, ?, ?, ?, ?, 'pending', NULL, ?)`,
		prepared.ID,
		prepared.CanonicalRevisionID,
		prepared.ExactCoreVersion,
		prepared.CoreArtifactID,
		prepared.ConfigBytes,
		prepared.ConfigSHA256,
		formatTime(prepared.CreatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return StartupArtifact{}, fmt.Errorf("%w: %s", ErrStartupArtifactExists, prepared.ID)
		}
		return StartupArtifact{}, fmt.Errorf("insert startup artifact: %w", err)
	}
	return getStartupArtifact(ctx, tx, prepared.ID)
}

func (s *Store) GetStartupArtifact(ctx context.Context, artifactID string) (StartupArtifact, error) {
	if strings.TrimSpace(artifactID) == "" {
		return StartupArtifact{}, errors.New("startup artifact id is empty")
	}
	return getStartupArtifact(ctx, s.db, artifactID)
}

// CompleteStartupArtifactCheck records exactly one checker outcome. A stale
// candidate is never revived by a late operation completion.
func (s *Store) CompleteStartupArtifactCheck(
	ctx context.Context,
	artifactID string,
	succeeded bool,
	checkedAt time.Time,
) (StartupArtifact, error) {
	if strings.TrimSpace(artifactID) == "" || checkedAt.IsZero() {
		return StartupArtifact{}, errors.New("startup artifact id and check time are required")
	}
	checkedAt = checkedAt.UTC()
	wanted := StartupArtifactFailed
	if succeeded {
		wanted = StartupArtifactReady
	}
	var stored StartupArtifact
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getStartupArtifact(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		switch current.State {
		case wanted:
			stored = current
			return nil
		case StartupArtifactPending:
		default:
			return fmt.Errorf("%w: %s is %s", ErrStartupArtifactState, artifactID, current.State)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE startup_artifacts
		            SET state = ?, checked_at = ?
		          WHERE id = ? AND state = 'pending'`,
			string(wanted),
			formatTime(checkedAt),
			artifactID,
		); err != nil {
			return fmt.Errorf("complete startup artifact check: %w", err)
		}
		stored, err = getStartupArtifact(ctx, tx, artifactID)
		return err
	})
	return stored, err
}

func prepareNewStartupArtifact(artifact StartupArtifact) (StartupArtifact, error) {
	if strings.TrimSpace(artifact.ID) == "" || strings.TrimSpace(artifact.CanonicalRevisionID) == "" ||
		strings.TrimSpace(artifact.CoreArtifactID) == "" {
		return StartupArtifact{}, errors.New("startup artifact identity fields are required")
	}
	version, err := coreartifact.ParseExactVersion(artifact.ExactCoreVersion)
	if err != nil || version.IsZero() {
		return StartupArtifact{}, errors.New("startup artifact exact version is invalid")
	}
	artifact.ExactCoreVersion = version.String()
	document, err := configuration.Parse(artifact.ConfigBytes)
	if err != nil {
		return StartupArtifact{}, fmt.Errorf("startup artifact config: %w", err)
	}
	artifact.ConfigBytes = document.CanonicalJSON()
	digest := sha256.Sum256(artifact.ConfigBytes)
	computedDigest := hex.EncodeToString(digest[:])
	if artifact.ConfigSHA256 != "" && artifact.ConfigSHA256 != computedDigest {
		return StartupArtifact{}, errors.New("startup artifact config digest does not match bytes")
	}
	artifact.ConfigSHA256 = computedDigest
	if artifact.State != "" && artifact.State != StartupArtifactPending {
		return StartupArtifact{}, errors.New("new startup artifact must be pending")
	}
	artifact.State = StartupArtifactPending
	artifact.CheckedAt = nil
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	} else {
		artifact.CreatedAt = artifact.CreatedAt.UTC()
	}
	artifact.ConfigBytes = bytes.Clone(artifact.ConfigBytes)
	return artifact, nil
}

func validateStartupArtifactReferences(ctx context.Context, tx *sql.Tx, artifact StartupArtifact) error {
	revision, err := getCanonicalRevision(tx.QueryRowContext(
		ctx,
		`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
		artifact.CanonicalRevisionID,
	))
	if err != nil {
		return err
	}
	if revision.SHA256 != artifact.ConfigSHA256 || !bytes.Equal(revision.Document, artifact.ConfigBytes) {
		return errors.New("startup artifact config does not match canonical revision")
	}
	core, err := getCoreArtifact(ctx, tx, artifact.CoreArtifactID)
	if err != nil {
		return err
	}
	if core.ExactVersion != artifact.ExactCoreVersion || core.ReportedVersion != artifact.ExactCoreVersion {
		return errors.New("startup artifact version does not match core artifact")
	}
	return nil
}

func getStartupArtifact(ctx context.Context, q queryRower, artifactID string) (StartupArtifact, error) {
	artifact, err := scanStartupArtifact(q.QueryRowContext(
		ctx,
		`SELECT `+startupArtifactColumns+` FROM startup_artifacts WHERE id = ?`,
		artifactID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return StartupArtifact{}, fmt.Errorf("%w: %s", ErrStartupArtifactNotFound, artifactID)
	}
	if err != nil {
		return StartupArtifact{}, fmt.Errorf("get startup artifact: %w", err)
	}
	return artifact, nil
}

func scanStartupArtifact(row rowScanner) (StartupArtifact, error) {
	var artifact StartupArtifact
	var checkedAt sql.NullString
	var createdAt string
	if err := row.Scan(
		&artifact.ID,
		&artifact.CanonicalRevisionID,
		&artifact.ExactCoreVersion,
		&artifact.CoreArtifactID,
		&artifact.ConfigBytes,
		&artifact.ConfigSHA256,
		&artifact.State,
		&checkedAt,
		&createdAt,
	); err != nil {
		return StartupArtifact{}, err
	}
	artifact.ConfigBytes = bytes.Clone(artifact.ConfigBytes)
	var err error
	artifact.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return StartupArtifact{}, fmt.Errorf("parse created_at: %w", err)
	}
	if checkedAt.Valid {
		parsed, err := parseTime(checkedAt.String)
		if err != nil {
			return StartupArtifact{}, fmt.Errorf("parse checked_at: %w", err)
		}
		artifact.CheckedAt = &parsed
	}
	return artifact, nil
}
