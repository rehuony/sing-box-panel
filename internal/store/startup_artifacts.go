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

type StartupArtifactKind string

const (
	StartupArtifactStructured StartupArtifactKind = "structured"
	StartupArtifactManual     StartupArtifactKind = "manual"
)

type StartupArtifactState string

const (
	StartupArtifactPending StartupArtifactState = "pending"
	StartupArtifactReady   StartupArtifactState = "ready"
	StartupArtifactFailed  StartupArtifactState = "failed"
	StartupArtifactStale   StartupArtifactState = "stale"
)

type StartupArtifact struct {
	ID                  string               `json:"id"`
	Kind                StartupArtifactKind  `json:"kind"`
	CanonicalRevisionID string               `json:"canonical_revision_id"`
	ExactCoreVersion    string               `json:"exact_core_version"`
	CapabilityCommit    string               `json:"capability_commit,omitempty"`
	CapabilityDigest    string               `json:"capability_digest,omitempty"`
	RendererVersion     string               `json:"renderer_version"`
	CoreArtifactID      string               `json:"core_artifact_id"`
	ConfigBytes         []byte               `json:"-"`
	ConfigSHA256        string               `json:"config_sha256"`
	Diagnostics         json.RawMessage      `json:"diagnostics"`
	State               StartupArtifactState `json:"state"`
	CheckedAt           *time.Time           `json:"checked_at,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

type StartupArtifactListFilter struct {
	CanonicalRevisionID string
	ExactCoreVersion    string
	CoreArtifactID      string
	Kind                StartupArtifactKind
	State               StartupArtifactState
	Cursor              *CreatedAtCursor
	Limit               int
}

type StartupArtifactPage struct {
	Items []StartupArtifact
	Next  *CreatedAtCursor
}

const startupArtifactColumns = `
    id, kind, canonical_revision_id, exact_core_version, capability_commit,
    capability_digest, renderer_version, core_artifact_id, config_bytes,
    config_sha256, diagnostics_json, state, checked_at, created_at`

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
                id, kind, canonical_revision_id, exact_core_version,
                capability_commit, capability_digest, renderer_version,
                core_artifact_id, config_bytes, config_sha256,
                diagnostics_json, state, checked_at, created_at
             ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', NULL, ?)`,
		prepared.ID,
		string(prepared.Kind),
		prepared.CanonicalRevisionID,
		prepared.ExactCoreVersion,
		nullIfEmpty(prepared.CapabilityCommit),
		nullIfEmpty(prepared.CapabilityDigest),
		prepared.RendererVersion,
		prepared.CoreArtifactID,
		prepared.ConfigBytes,
		prepared.ConfigSHA256,
		string(prepared.Diagnostics),
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
	if filter.Kind != "" && !validStartupArtifactKind(filter.Kind) {
		return StartupArtifactPage{}, fmt.Errorf("invalid startup artifact kind %q", filter.Kind)
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
		{string(filter.Kind), "kind = ?"},
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
		`SELECT `+startupArtifactColumns+` FROM startup_artifacts
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY created_at DESC, id DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return StartupArtifactPage{}, fmt.Errorf("list startup artifacts: %w", err)
	}
	defer rows.Close()
	items := make([]StartupArtifact, 0, limit+1)
	for rows.Next() {
		artifact, err := scanStartupArtifact(rows)
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
		case StartupArtifactStale:
			stored = current
			return nil
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

// MarkStartupArtifactStale discards a pending or ready immutable candidate.
// Applied bundles keep their own reference and bytes; this state transition
// only prevents preparing a new activation from the candidate.
func (s *Store) MarkStartupArtifactStale(
	ctx context.Context,
	artifactID string,
) (StartupArtifact, error) {
	if strings.TrimSpace(artifactID) == "" {
		return StartupArtifact{}, errors.New("startup artifact id is empty")
	}
	var stored StartupArtifact
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getStartupArtifact(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		if current.State == StartupArtifactStale {
			stored = current
			return nil
		}
		if current.State != StartupArtifactPending && current.State != StartupArtifactReady &&
			current.State != StartupArtifactFailed {
			return fmt.Errorf("%w: %s is %s", ErrStartupArtifactState, artifactID, current.State)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE startup_artifacts SET state = 'stale' WHERE id = ?`,
			artifactID,
		); err != nil {
			return fmt.Errorf("mark startup artifact stale: %w", err)
		}
		stored, err = getStartupArtifact(ctx, tx, artifactID)
		return err
	})
	return stored, err
}

func prepareNewStartupArtifact(artifact StartupArtifact) (StartupArtifact, error) {
	if strings.TrimSpace(artifact.ID) == "" || strings.TrimSpace(artifact.CanonicalRevisionID) == "" ||
		strings.TrimSpace(artifact.RendererVersion) == "" || strings.TrimSpace(artifact.CoreArtifactID) == "" {
		return StartupArtifact{}, errors.New("startup artifact identity fields are required")
	}
	if !validStartupArtifactKind(artifact.Kind) {
		return StartupArtifact{}, fmt.Errorf("invalid startup artifact kind %q", artifact.Kind)
	}
	version, err := coreartifact.ParseExactVersion(artifact.ExactCoreVersion)
	if err != nil || version.IsZero() {
		return StartupArtifact{}, errors.New("startup artifact exact version is invalid")
	}
	artifact.ExactCoreVersion = version.String()
	if artifact.Kind == StartupArtifactStructured {
		if strings.TrimSpace(artifact.CapabilityCommit) == "" {
			return StartupArtifact{}, errors.New("structured startup artifact capability commit is required")
		}
		digest, err := coreartifact.ParseSHA256(artifact.CapabilityDigest)
		if err != nil || digest.IsZero() {
			return StartupArtifact{}, errors.New("structured startup artifact capability digest is invalid")
		}
		artifact.CapabilityDigest = digest.String()
	} else if artifact.CapabilityCommit != "" || artifact.CapabilityDigest != "" {
		return StartupArtifact{}, errors.New("manual startup artifact cannot claim a capability manifest")
	}
	if len(artifact.ConfigBytes) == 0 || len(artifact.ConfigBytes) > 4<<20 {
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
	var capabilityCommit, capabilityDigest, checkedAt sql.NullString
	var diagnostics, createdAt string
	if err := row.Scan(
		&artifact.ID,
		&artifact.Kind,
		&artifact.CanonicalRevisionID,
		&artifact.ExactCoreVersion,
		&capabilityCommit,
		&capabilityDigest,
		&artifact.RendererVersion,
		&artifact.CoreArtifactID,
		&artifact.ConfigBytes,
		&artifact.ConfigSHA256,
		&diagnostics,
		&artifact.State,
		&checkedAt,
		&createdAt,
	); err != nil {
		return StartupArtifact{}, err
	}
	artifact.CapabilityCommit = valueOrEmpty(capabilityCommit)
	artifact.CapabilityDigest = valueOrEmpty(capabilityDigest)
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

func validStartupArtifactKind(value StartupArtifactKind) bool {
	return value == StartupArtifactStructured || value == StartupArtifactManual
}

func validStartupArtifactState(value StartupArtifactState) bool {
	switch value {
	case StartupArtifactPending, StartupArtifactReady, StartupArtifactFailed, StartupArtifactStale:
		return true
	default:
		return false
	}
}
