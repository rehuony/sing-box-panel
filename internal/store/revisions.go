package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

var ErrRevisionConflict = errors.New("canonical revision head conflict")

// RevisionConflictError reports a failed compare-and-swap without hiding the
// actual head an interactive client needs for a three-way diff.
type RevisionConflictError struct {
	ExpectedHead string
	ActualHead   string
}

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf(
		"%v: expected head %q, actual head %q",
		ErrRevisionConflict,
		e.ExpectedHead,
		e.ActualHead,
	)
}

func (e *RevisionConflictError) Unwrap() error {
	return ErrRevisionConflict
}

// CanonicalRevision is one immutable full snapshot of canonical configuration.
type CanonicalRevision struct {
	ID            string
	Sequence      int64
	ParentID      string
	SchemaVersion int
	Document      json.RawMessage
	SHA256        string
	CommandID     string
	CreatedAt     time.Time
}

// NewCanonicalRevision contains caller-owned identity and canonical bytes. The
// store derives the parent, sequence, and digest inside the CAS operation.
type NewCanonicalRevision struct {
	ID            string
	SchemaVersion int
	Document      json.RawMessage
	CommandID     string
	CreatedAt     time.Time
}

type TaskLane string

const (
	TaskLaneRuntime     TaskLane = "runtime"
	TaskLaneMaintenance TaskLane = "maintenance"
)

// NewTask is the durable work enqueued atomically with a canonical save.
type NewTask struct {
	ID             string
	IdempotencyKey string
	Lane           TaskLane
	Kind           TaskKind
	Generation     int64
	Payload        json.RawMessage
	CreatedAt      time.Time
}

// HubState is the singleton set of mutable control-plane pointers.
type HubState struct {
	HeadRevisionID   string
	DesiredBundleID  string
	AppliedBundleID  string
	RollbackBundleID string
	AppliedAt        *time.Time
	TargetGeneration int64
	DesiredRunning   bool
	UpdatedAt        time.Time
}

// BootstrapState is the minimum consistent state needed at process startup.
type BootstrapState struct {
	Schema SchemaInfo
	Hub    HubState
	Head   *CanonicalRevision
}

// SaveCanonicalRevisionAndTask creates one immutable full snapshot, advances
// the singleton head with compare-and-swap semantics, and enqueues its durable
// task in the same short SQLite transaction.
func (s *Store) SaveCanonicalRevisionAndTask(
	ctx context.Context,
	expectedHead string,
	revision NewCanonicalRevision,
	task NewTask,
) (CanonicalRevision, error) {
	preparedRevision, preparedTask, err := prepareCanonicalSave(revision, task)
	if err != nil {
		return CanonicalRevision{}, err
	}

	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		storedRevision, _, err := saveCanonicalRevisionTx(
			ctx,
			tx,
			expectedHead,
			preparedRevision,
			false,
		)
		if err != nil {
			return err
		}
		preparedRevision = storedRevision
		return insertCanonicalTaskTx(ctx, tx, preparedTask, preparedRevision.ID, "")
	})
	if err != nil {
		return CanonicalRevision{}, err
	}

	return preparedRevision, nil
}

// saveCanonicalRevisionTx checks expectedHead and creates a new immutable
// revision. When reuseUnchanged is true, a byte-identical document reuses the
// current head after the same compare-and-swap check. The returned bool reports
// whether a new revision was inserted.
func saveCanonicalRevisionTx(
	ctx context.Context,
	tx *sql.Tx,
	expectedHead string,
	prepared CanonicalRevision,
	reuseUnchanged bool,
) (CanonicalRevision, bool, error) {
	var currentHead sql.NullString
	if err := tx.QueryRowContext(
		ctx,
		`SELECT head_revision_id FROM hub_state WHERE singleton = 1`,
	).Scan(&currentHead); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CanonicalRevision{}, false, fmt.Errorf(
				"%w: singleton hub_state row is missing",
				ErrSchemaInconsistent,
			)
		}
		return CanonicalRevision{}, false, fmt.Errorf("read canonical head: %w", err)
	}

	actualHead := valueOrEmpty(currentHead)
	if actualHead != expectedHead {
		return CanonicalRevision{}, false, &RevisionConflictError{
			ExpectedHead: expectedHead,
			ActualHead:   actualHead,
		}
	}

	sequence := int64(1)
	if currentHead.Valid {
		head, err := getCanonicalRevision(tx.QueryRowContext(
			ctx,
			`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
			currentHead.String,
		))
		if err != nil {
			return CanonicalRevision{}, false, fmt.Errorf("read canonical head revision: %w", err)
		}
		if reuseUnchanged && head.SHA256 == prepared.SHA256 {
			return head, false, nil
		}
		sequence = head.Sequence + 1
	}

	prepared.Sequence = sequence
	prepared.ParentID = actualHead
	createdAt := prepared.CreatedAt.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO canonical_revisions(
            id, sequence, parent_id, schema_version, document_json,
            sha256, command_id, created_at
         ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		prepared.ID,
		prepared.Sequence,
		nullIfEmpty(prepared.ParentID),
		prepared.SchemaVersion,
		string(prepared.Document),
		prepared.SHA256,
		prepared.CommandID,
		createdAt,
	); err != nil {
		return CanonicalRevision{}, false, fmt.Errorf("insert canonical revision: %w", err)
	}

	result, err := tx.ExecContext(
		ctx,
		`UPDATE hub_state
            SET head_revision_id = ?, updated_at = ?
          WHERE singleton = 1 AND head_revision_id IS ?`,
		prepared.ID,
		createdAt,
		nullIfEmpty(expectedHead),
	)
	if err != nil {
		return CanonicalRevision{}, false, fmt.Errorf("advance canonical head: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return CanonicalRevision{}, false, fmt.Errorf("inspect canonical head update: %w", err)
	}
	if rows != 1 {
		return CanonicalRevision{}, false, &RevisionConflictError{
			ExpectedHead: expectedHead,
			ActualHead:   actualHead,
		}
	}
	return prepared, true, nil
}

func insertCanonicalTaskTx(
	ctx context.Context,
	tx *sql.Tx,
	task NewTask,
	canonicalRevisionID string,
	startupArtifactID string,
) error {
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO tasks(
            id, idempotency_key, lane, kind, status, generation,
            canonical_revision_id, startup_artifact_id, payload_json,
            created_at, updated_at
         ) VALUES (?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?)`,
		task.ID,
		nullIfEmpty(task.IdempotencyKey),
		string(task.Lane),
		task.Kind,
		task.Generation,
		canonicalRevisionID,
		nullIfEmpty(startupArtifactID),
		string(task.Payload),
		formatTaskTime(task.CreatedAt),
		formatTaskTime(task.CreatedAt),
	); err != nil {
		return fmt.Errorf("enqueue canonical task: %w", err)
	}
	return nil
}

func prepareCanonicalSave(
	revision NewCanonicalRevision,
	task NewTask,
) (CanonicalRevision, NewTask, error) {
	if strings.TrimSpace(revision.ID) == "" {
		return CanonicalRevision{}, NewTask{}, errors.New("canonical revision id is empty")
	}
	if revision.SchemaVersion != configuration.SchemaVersion {
		return CanonicalRevision{}, NewTask{}, fmt.Errorf(
			"canonical schema version must be exactly %d",
			configuration.SchemaVersion,
		)
	}
	document, err := configuration.Parse(revision.Document)
	if err != nil {
		return CanonicalRevision{}, NewTask{}, fmt.Errorf("canonical document: %w", err)
	}
	revision.Document = document.CanonicalJSON()
	if strings.TrimSpace(revision.CommandID) == "" {
		return CanonicalRevision{}, NewTask{}, errors.New("canonical command id is empty")
	}
	if revision.CreatedAt.IsZero() {
		revision.CreatedAt = time.Now().UTC()
	} else {
		revision.CreatedAt = revision.CreatedAt.UTC()
	}

	if strings.TrimSpace(task.ID) == "" {
		return CanonicalRevision{}, NewTask{}, errors.New("task id is empty")
	}
	if task.Lane != TaskLaneRuntime && task.Lane != TaskLaneMaintenance {
		return CanonicalRevision{}, NewTask{}, fmt.Errorf("invalid task lane %q", task.Lane)
	}
	if !validTaskLaneKind(task.Lane, task.Kind) {
		return CanonicalRevision{}, NewTask{}, fmt.Errorf("invalid %s task kind %q", task.Lane, task.Kind)
	}
	if task.Generation < 0 {
		return CanonicalRevision{}, NewTask{}, errors.New("task generation must not be negative")
	}
	if len(task.Payload) == 0 {
		task.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(task.Payload) {
		return CanonicalRevision{}, NewTask{}, errors.New("task payload is not valid JSON")
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = revision.CreatedAt
	} else {
		task.CreatedAt = task.CreatedAt.UTC()
	}

	digest := sha256.Sum256(revision.Document)
	return CanonicalRevision{
		ID:            revision.ID,
		SchemaVersion: revision.SchemaVersion,
		Document:      append(json.RawMessage(nil), revision.Document...),
		SHA256:        hex.EncodeToString(digest[:]),
		CommandID:     revision.CommandID,
		CreatedAt:     revision.CreatedAt,
	}, task, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Bootstrap reads schema identity and the singleton hub/head snapshot.
func (s *Store) Bootstrap(ctx context.Context) (BootstrapState, error) {
	schema, err := s.SchemaInfo(ctx)
	if err != nil {
		return BootstrapState{}, err
	}

	var (
		headID           sql.NullString
		desiredBundle    sql.NullString
		appliedBundle    sql.NullString
		rollbackBundle   sql.NullString
		appliedAt        sql.NullString
		targetGeneration int64
		desiredRunning   int
		hubUpdatedAt     string

		revisionID       sql.NullString
		revisionSequence sql.NullInt64
		revisionParent   sql.NullString
		revisionSchema   sql.NullInt64
		revisionDocument sql.NullString
		revisionSHA      sql.NullString
		revisionCommand  sql.NullString
		revisionCreated  sql.NullString
	)

	err = s.db.QueryRowContext(
		ctx,
		`SELECT
            h.head_revision_id, h.desired_bundle_id, h.applied_bundle_id,
            h.rollback_bundle_id, h.applied_at, h.target_generation, h.desired_running,
            h.updated_at,
            r.id, r.sequence, r.parent_id, r.schema_version, r.document_json,
            r.sha256, r.command_id, r.created_at
         FROM hub_state AS h
         LEFT JOIN canonical_revisions AS r ON r.id = h.head_revision_id
         WHERE h.singleton = 1`,
	).Scan(
		&headID,
		&desiredBundle,
		&appliedBundle,
		&rollbackBundle,
		&appliedAt,
		&targetGeneration,
		&desiredRunning,
		&hubUpdatedAt,
		&revisionID,
		&revisionSequence,
		&revisionParent,
		&revisionSchema,
		&revisionDocument,
		&revisionSHA,
		&revisionCommand,
		&revisionCreated,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BootstrapState{}, fmt.Errorf("%w: singleton hub_state row is missing", ErrSchemaInconsistent)
		}
		return BootstrapState{}, fmt.Errorf("read SQLite bootstrap state: %w", err)
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, hubUpdatedAt)
	if err != nil {
		return BootstrapState{}, fmt.Errorf("parse hub_state updated_at: %w", err)
	}
	state := BootstrapState{
		Schema: schema,
		Hub: HubState{
			HeadRevisionID:   valueOrEmpty(headID),
			DesiredBundleID:  valueOrEmpty(desiredBundle),
			AppliedBundleID:  valueOrEmpty(appliedBundle),
			RollbackBundleID: valueOrEmpty(rollbackBundle),
			TargetGeneration: targetGeneration,
			DesiredRunning:   desiredRunning != 0,
			UpdatedAt:        updatedAt,
		},
	}
	if appliedAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, appliedAt.String)
		if parseErr != nil {
			return BootstrapState{}, fmt.Errorf("parse hub_state applied_at: %w", parseErr)
		}
		parsed = parsed.UTC()
		state.Hub.AppliedAt = &parsed
	}

	if revisionID.Valid {
		createdAt, err := time.Parse(time.RFC3339Nano, revisionCreated.String)
		if err != nil {
			return BootstrapState{}, fmt.Errorf("parse canonical revision created_at: %w", err)
		}
		state.Head = &CanonicalRevision{
			ID:            revisionID.String,
			Sequence:      revisionSequence.Int64,
			ParentID:      valueOrEmpty(revisionParent),
			SchemaVersion: int(revisionSchema.Int64),
			Document:      json.RawMessage(revisionDocument.String),
			SHA256:        revisionSHA.String,
			CommandID:     revisionCommand.String,
			CreatedAt:     createdAt,
		}
	}

	return state, nil
}

// Head returns the current immutable canonical revision, or nil before the
// first successful save.
func (s *Store) Head(ctx context.Context) (*CanonicalRevision, error) {
	state, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	return state.Head, nil
}

func valueOrEmpty(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
