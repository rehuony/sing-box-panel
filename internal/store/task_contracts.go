// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTaskNotFound           = errors.New("task not found")
	ErrTaskLeaseLost          = errors.New("task lease lost")
	ErrTaskGenerationConflict = errors.New("runtime task generation conflict")
)

type TaskStatus string

const (
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusSucceeded  TaskStatus = "succeeded"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCanceled   TaskStatus = "canceled"
	TaskStatusSuperseded TaskStatus = "superseded"
)

// TaskKind identifies every durable operation understood by this binary.
// Keeping this list closed prevents producers from persisting work for which
// the server has no handler.
type TaskKind string

const (
	TaskKindCanonicalSaved            TaskKind = "canonical-saved"
	TaskKindCatalogRefresh            TaskKind = "catalog-refresh"
	TaskKindCoreInstall               TaskKind = "core-install"
	TaskKindCoreImport                TaskKind = "core-import"
	TaskKindStartupCheck              TaskKind = "startup-check"
	TaskKindSubscriptionSourceRefresh TaskKind = "subscription-source-refresh"
	TaskKindRuntimeApply              TaskKind = "runtime-apply"
	TaskKindRuntimeStart              TaskKind = "runtime-start"
	TaskKindRuntimeStop               TaskKind = "runtime-stop"
	TaskKindRuntimeRestart            TaskKind = "runtime-restart"
	TaskKindRuntimeRollback           TaskKind = "runtime-rollback"
)

// BuiltInTaskKinds returns the closed task contract in stable order.
func BuiltInTaskKinds() []TaskKind {
	return []TaskKind{
		TaskKindCanonicalSaved,
		TaskKindCatalogRefresh,
		TaskKindCoreInstall,
		TaskKindCoreImport,
		TaskKindStartupCheck,
		TaskKindSubscriptionSourceRefresh,
		TaskKindRuntimeApply,
		TaskKindRuntimeStart,
		TaskKindRuntimeStop,
		TaskKindRuntimeRestart,
		TaskKindRuntimeRollback,
	}
}

// Task is one durable unit of runtime or maintenance work.
type Task struct {
	ID                  string
	IdempotencyKey      string
	Lane                TaskLane
	Kind                TaskKind
	Status              TaskStatus
	Generation          int64
	CanonicalRevisionID string
	StartupArtifactID   string
	ActivationBundleID  string
	Payload             json.RawMessage
	Result              json.RawMessage
	Failure             json.RawMessage
	CancelRequested     bool
	Attempt             int
	LeaseOwner          string
	LeaseExpiresAt      *time.Time
	NotBefore           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// EnqueueTaskInput contains task data prepared outside the transaction.
// Runtime tasks are intents: their generation must be newer than hub_state's
// target_generation and atomically supersedes older runtime work.
type EnqueueTaskInput struct {
	ID                  string
	IdempotencyKey      string
	Lane                TaskLane
	Kind                TaskKind
	Generation          int64
	CanonicalRevisionID string
	StartupArtifactID   string
	ActivationBundleID  string
	Payload             json.RawMessage
	NotBefore           *time.Time
	CreatedAt           time.Time
}

// ClaimTaskInput identifies one lane worker and the lease it is requesting.
// LeaseOwner is a human-readable prefix; the returned Task.LeaseOwner is a
// unique fencing token and must be used for heartbeat and completion.
type ClaimTaskInput struct {
	Lane          TaskLane
	LeaseOwner    string
	Now           time.Time
	LeaseDuration time.Duration
}

// TaskLeaseState is returned at each heartbeat/safe cancellation boundary.
type TaskLeaseState struct {
	CancelRequested bool
	Superseded      bool
	LeaseExpiresAt  time.Time
}

// TaskCompletion is the handler result. Cancellation and generation
// supersession take precedence over this requested result.
type TaskCompletion struct {
	Succeeded bool
	Result    json.RawMessage
	Failure   json.RawMessage
}

type taskScanner interface {
	Scan(...any) error
}

const taskColumns = `
    id, idempotency_key, lane, kind, status, generation,
    canonical_revision_id, startup_artifact_id, activation_bundle_id,
    payload_json, result_json, error_json, cancel_requested, attempt,
    lease_owner, lease_expires_at, not_before, created_at, updated_at`

// EnqueueTask inserts durable work. A runtime task also advances the desired
// generation, terminally supersedes older queued intents, and asks an older
// running intent to stop at its next safe cancellation boundary.

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func nullableTaskTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTaskTime(*value)
}

func formatTaskTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTaskTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newLeaseOwner(prefix string) (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate task lease token: %w", err)
	}
	return prefix + "/" + hex.EncodeToString(token[:]), nil
}
