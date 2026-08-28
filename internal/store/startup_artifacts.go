// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	AdapterID           string               `json:"adapter_id"`
	AdapterRevision     string               `json:"adapter_revision"`
	CoreArtifactID      string               `json:"core_artifact_id"`
	ConfigBytes         []byte               `json:"-"`
	ConfigSHA256        string               `json:"config_sha256"`
	Diagnostics         json.RawMessage      `json:"diagnostics"`
	IgnoredDigest       string               `json:"ignored_digest,omitempty"`
	State               StartupArtifactState `json:"state"`
	CheckedAt           *time.Time           `json:"checked_at,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

// StartupArtifactSummary contains only metadata safe for collection queries.
// ConfigBytes remains available exclusively through GetStartupArtifact.
type StartupArtifactSummary struct {
	ID                  string
	CanonicalRevisionID string
	ExactCoreVersion    string
	AdapterID           string
	AdapterRevision     string
	CoreArtifactID      string
	ConfigSHA256        string
	Diagnostics         json.RawMessage
	IgnoredDigest       string
	State               StartupArtifactState
	CheckedAt           *time.Time
	CreatedAt           time.Time
}

type StartupArtifactListFilter struct {
	CanonicalRevisionID string
	ExactCoreVersion    string
	CoreArtifactID      string
	State               StartupArtifactState
	Cursor              *CreatedAtCursor
	Limit               int
}

type StartupArtifactPage struct {
	Items []StartupArtifactSummary
	Next  *CreatedAtCursor
}

const startupArtifactColumns = `
	    id, canonical_revision_id, exact_core_version, adapter_id,
	    adapter_revision, core_artifact_id, config_bytes, config_sha256,
	    diagnostics_json, ignored_digest, state, checked_at, created_at`

const startupArtifactSummaryColumns = `
	    id, canonical_revision_id, exact_core_version, adapter_id,
	    adapter_revision, core_artifact_id, config_sha256, diagnostics_json,
	    ignored_digest, state, checked_at, created_at`

// CreateStartupArtifact inserts an immutable pending candidate. Only the
// durable checker may transition it to ready or failed.
func (s *Store) CreateStartupArtifact(
	ctx context.Context,
	artifact StartupArtifact,
) (StartupArtifact, error) {
	prepared, err := prepareNewStartupArtifact(artifact)
	if err != nil {
		return StartupArtifact{}, err
	}
	var stored StartupArtifact
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		stored, err = insertStartupArtifactTx(ctx, tx, prepared)
		return err
	})
	return stored, err
}

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
	                id, canonical_revision_id, exact_core_version, adapter_id,
	                adapter_revision, core_artifact_id, config_bytes, config_sha256,
	                diagnostics_json, ignored_digest, state, checked_at, created_at
	             ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', NULL, ?)`,
		prepared.ID,
		prepared.CanonicalRevisionID,
		prepared.ExactCoreVersion,
		prepared.AdapterID,
		prepared.AdapterRevision,
		prepared.CoreArtifactID,
		prepared.ConfigBytes,
		prepared.ConfigSHA256,
		string(prepared.Diagnostics),
		nullIfEmpty(prepared.IgnoredDigest),
		formatTaskTime(prepared.CreatedAt),
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

func (s *Store) ListStartupArtifacts(
	ctx context.Context,
	filter StartupArtifactListFilter,
) (StartupArtifactPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return StartupArtifactPage{}, err
	}
	if filter.State != "" && !validStartupArtifactState(filter.State) {
		return StartupArtifactPage{}, fmt.Errorf("invalid startup artifact state %q", filter.State)
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return StartupArtifactPage{}, err
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 12)
	for _, item := range []struct {
		value  string
		clause string
	}{
		{filter.CanonicalRevisionID, "canonical_revision_id = ?"},
		{filter.ExactCoreVersion, "exact_core_version = ?"},
		{filter.CoreArtifactID, "core_artifact_id = ?"},
		{string(filter.State), "state = ?"},
	} {
		if item.value != "" {
			clauses = append(clauses, item.clause)
			args = append(args, item.value)
		}
	}
	if filter.Cursor != nil {
		cursorTime := formatTaskTime(filter.Cursor.CreatedAt)
		clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+startupArtifactSummaryColumns+` FROM startup_artifacts
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY created_at DESC, id DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return StartupArtifactPage{}, fmt.Errorf("list startup artifacts: %w", err)
	}
	defer rows.Close()
	items := make([]StartupArtifactSummary, 0, limit+1)
	for rows.Next() {
		artifact, err := scanStartupArtifactSummary(rows)
		if err != nil {
			return StartupArtifactPage{}, fmt.Errorf("scan startup artifact: %w", err)
		}
		items = append(items, artifact)
	}
	if err := rows.Err(); err != nil {
		return StartupArtifactPage{}, fmt.Errorf("iterate startup artifacts: %w", err)
	}
	page := StartupArtifactPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// CompleteStartupArtifactCheck records exactly one checker outcome. A stale
// candidate is never revived by a late task completion.
func (s *Store) CompleteStartupArtifactCheck(
	ctx context.Context,
	artifactID string,
	succeeded bool,
	diagnostics json.RawMessage,
	checkedAt time.Time,
) (StartupArtifact, error) {
	if strings.TrimSpace(artifactID) == "" || checkedAt.IsZero() {
		return StartupArtifact{}, errors.New("startup artifact id and check time are required")
	}
	diagnostics, err := compactJSONArray(diagnostics)
	if err != nil {
		return StartupArtifact{}, err
	}
	checkedAt = checkedAt.UTC()
	wanted := StartupArtifactFailed
	if succeeded {
		wanted = StartupArtifactReady
	}
	var stored StartupArtifact
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
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
                    SET state = ?, diagnostics_json = ?, checked_at = ?
                  WHERE id = ? AND state = 'pending'`,
			string(wanted),
			string(diagnostics),
			formatTaskTime(checkedAt),
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
		strings.TrimSpace(artifact.AdapterID) == "" || strings.TrimSpace(artifact.AdapterRevision) == "" ||
		strings.TrimSpace(artifact.CoreArtifactID) == "" {
		return StartupArtifact{}, errors.New("startup artifact identity fields are required")
	}
	version, err := coreartifact.ParseExactVersion(artifact.ExactCoreVersion)
	if err != nil || version.IsZero() {
		return StartupArtifact{}, errors.New("startup artifact exact version is invalid")
	}
	artifact.ExactCoreVersion = version.String()
	if len(artifact.ConfigBytes) == 0 || len(artifact.ConfigBytes) > 4<<20 || !json.Valid(artifact.ConfigBytes) {
		return StartupArtifact{}, errors.New("startup artifact config bytes are empty or too large")
	}
	digest := sha256.Sum256(artifact.ConfigBytes)
	computedDigest := hex.EncodeToString(digest[:])
	if artifact.ConfigSHA256 != "" && artifact.ConfigSHA256 != computedDigest {
		return StartupArtifact{}, errors.New("startup artifact config digest does not match bytes")
	}
	artifact.ConfigSHA256 = computedDigest
	artifact.Diagnostics, err = compactJSONArray(artifact.Diagnostics)
	if err != nil {
		return StartupArtifact{}, err
	}
	if artifact.IgnoredDigest != "" {
		digest, digestErr := coreartifact.ParseSHA256(artifact.IgnoredDigest)
		if digestErr != nil || digest.IsZero() {
			return StartupArtifact{}, errors.New("startup artifact ignored digest is invalid")
		}
		artifact.IgnoredDigest = digest.String()
	}
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
	if _, err := getCanonicalRevision(tx.QueryRowContext(
		ctx,
		`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
		artifact.CanonicalRevisionID,
	)); err != nil {
		return err
	}
	core, err := getCoreArtifact(ctx, tx, artifact.CoreArtifactID)
	if err != nil {
		return err
	}
	if core.ExactVersion != artifact.ExactCoreVersion || core.ReportedVersion != artifact.ExactCoreVersion {
		return errors.New("startup artifact version does not match core artifact")
	}
	if core.VerificationState != CoreArtifactVerified {
		return errors.New("core artifact is not eligible for new startup work")
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

func scanStartupArtifact(row taskScanner) (StartupArtifact, error) {
	var artifact StartupArtifact
	var ignoredDigest, checkedAt sql.NullString
	var diagnostics, createdAt string
	if err := row.Scan(
		&artifact.ID,
		&artifact.CanonicalRevisionID,
		&artifact.ExactCoreVersion,
		&artifact.AdapterID,
		&artifact.AdapterRevision,
		&artifact.CoreArtifactID,
		&artifact.ConfigBytes,
		&artifact.ConfigSHA256,
		&diagnostics,
		&ignoredDigest,
		&artifact.State,
		&checkedAt,
		&createdAt,
	); err != nil {
		return StartupArtifact{}, err
	}
	artifact.IgnoredDigest = valueOrEmpty(ignoredDigest)
	artifact.ConfigBytes = bytes.Clone(artifact.ConfigBytes)
	artifact.Diagnostics = append(json.RawMessage(nil), diagnostics...)
	var err error
	artifact.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return StartupArtifact{}, fmt.Errorf("parse created_at: %w", err)
	}
	if checkedAt.Valid {
		parsed, err := parseTaskTime(checkedAt.String)
		if err != nil {
			return StartupArtifact{}, fmt.Errorf("parse checked_at: %w", err)
		}
		artifact.CheckedAt = &parsed
	}
	return artifact, nil
}

func scanStartupArtifactSummary(row taskScanner) (StartupArtifactSummary, error) {
	var artifact StartupArtifactSummary
	var ignoredDigest, checkedAt sql.NullString
	var diagnostics, createdAt string
	if err := row.Scan(
		&artifact.ID,
		&artifact.CanonicalRevisionID,
		&artifact.ExactCoreVersion,
		&artifact.AdapterID,
		&artifact.AdapterRevision,
		&artifact.CoreArtifactID,
		&artifact.ConfigSHA256,
		&diagnostics,
		&ignoredDigest,
		&artifact.State,
		&checkedAt,
		&createdAt,
	); err != nil {
		return StartupArtifactSummary{}, err
	}
	artifact.IgnoredDigest = valueOrEmpty(ignoredDigest)
	artifact.Diagnostics = append(json.RawMessage(nil), diagnostics...)
	var err error
	artifact.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return StartupArtifactSummary{}, fmt.Errorf("parse created_at: %w", err)
	}
	if checkedAt.Valid {
		parsed, err := parseTaskTime(checkedAt.String)
		if err != nil {
			return StartupArtifactSummary{}, fmt.Errorf("parse checked_at: %w", err)
		}
		artifact.CheckedAt = &parsed
	}
	return artifact, nil
}

func compactJSONArray(value json.RawMessage) (json.RawMessage, error) {
	compacted, err := compactJSON(value, "[]")
	if err != nil {
		return nil, err
	}
	var array []any
	if err := json.Unmarshal(compacted, &array); err != nil || array == nil {
		return nil, errors.New("diagnostics must be a JSON array")
	}
	return compacted, nil
}

func validStartupArtifactState(value StartupArtifactState) bool {
	switch value {
	case StartupArtifactPending, StartupArtifactReady, StartupArtifactFailed:
		return true
	default:
		return false
	}
}
