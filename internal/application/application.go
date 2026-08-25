// SPDX-License-Identifier: GPL-3.0-or-later

// Package application exposes use cases shared by CLI and HTTP transports.
package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrCanonicalPatchInvalid = errors.New("canonical patch is invalid")

// Application owns one short-lived CLI database handle or the server's shared
// handle. It contains no transport-specific behavior.
type Application struct {
	database     *store.Store
	ownsDatabase bool
	now          func() time.Time
	random       func([]byte) (int, error)
	runtime      RuntimeResolver
	settings     settings.Settings
}

// RuntimeResolver resolves the verified identity of the currently running
// sing-box process. It is exported within this internal package boundary so
// controlled composition and transport tests can replace only this dependency.
type RuntimeResolver interface {
	Resolve(context.Context) (runtimeidentity.Identity, error)
}

type CoreVersionResolution struct {
	ExactVersion string                    `json:"exact_version"`
	Source       string                    `json:"source"`
	Running      *runtimeidentity.Identity `json:"running,omitempty"`
}

type CanonicalSnapshot struct {
	ID            string          `json:"id"`
	Sequence      int64           `json:"sequence"`
	ParentID      string          `json:"parent_id,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Document      json.RawMessage `json:"document"`
	DocumentJSON  string          `json:"document_json"`
	SHA256        string          `json:"sha256"`
	CreatedAt     time.Time       `json:"created_at"`
}

type CanonicalChange struct {
	Operation string `json:"op"`
	Path      string `json:"path"`
	ValueJSON string `json:"value_json,omitempty"`
}

type CanonicalSave struct {
	Revision CanonicalSnapshot `json:"revision"`
	TaskID   string            `json:"task_id,omitempty"`
	NoChange bool              `json:"no_change"`
}

type CanonicalValue struct {
	Revision CanonicalSnapshot `json:"revision"`
	Pointer  string            `json:"pointer"`
	Value    any               `json:"value"`
}

type CanonicalRevisionPage struct {
	Items []CanonicalSnapshot `json:"items"`
	Next  *int64              `json:"next_before_sequence,omitempty"`
}

type DiffValue struct {
	Present bool `json:"present"`
	Value   any  `json:"value,omitempty"`
}

type CanonicalDiffEntry struct {
	Path string    `json:"path"`
	From DiffValue `json:"from"`
	To   DiffValue `json:"to"`
}

type CanonicalRevisionDiff struct {
	From    CanonicalSnapshot    `json:"from"`
	To      CanonicalSnapshot    `json:"to"`
	Changes []CanonicalDiffEntry `json:"changes"`
}

type Task struct {
	ID                  string           `json:"id"`
	IdempotencyKey      string           `json:"idempotency_key,omitempty"`
	Lane                store.TaskLane   `json:"lane"`
	Kind                string           `json:"kind"`
	Status              store.TaskStatus `json:"status"`
	Generation          int64            `json:"generation"`
	CanonicalRevisionID string           `json:"canonical_revision_id,omitempty"`
	StartupArtifactID   string           `json:"startup_artifact_id,omitempty"`
	ActivationBundleID  string           `json:"activation_bundle_id,omitempty"`
	Payload             json.RawMessage  `json:"payload"`
	Result              json.RawMessage  `json:"result,omitempty"`
	Failure             json.RawMessage  `json:"failure,omitempty"`
	CancelRequested     bool             `json:"cancel_requested"`
	Attempt             int              `json:"attempt"`
	LeaseExpiresAt      *time.Time       `json:"lease_expires_at,omitempty"`
	NotBefore           *time.Time       `json:"not_before,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

func (task Task) Terminal() bool {
	switch task.Status {
	case store.TaskStatusSucceeded, store.TaskStatusFailed, store.TaskStatusCanceled, store.TaskStatusSuperseded:
		return true
	default:
		return false
	}
}

type TaskListFilter struct {
	Lane   store.TaskLane
	Status store.TaskStatus
	Kind   string
	Cursor *store.CreatedAtCursor
	Limit  int
}

type TaskPage struct {
	Items []Task      `json:"items"`
	Next  *TaskCursor `json:"next,omitempty"`
}

type TaskCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// Open resolves bootstrap settings and opens the SQLite source of truth.
func Open(ctx context.Context, settingsPath string) (*Application, error) {
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(configuration.DataDir)
	if err != nil {
		return nil, fmt.Errorf("inspect data directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("data path is not a directory: %s", configuration.DataDir)
	}
	database, err := store.Open(ctx, filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		return nil, err
	}
	application := newApplication(database)
	application.ownsDatabase = true
	application.settings = configuration
	return application, nil
}

func newApplication(database *store.Store) *Application {
	return &Application{
		database: database,
		now:      time.Now,
		random:   rand.Read,
		runtime:  runtimeidentity.New(database),
	}
}

// FromStore exposes application use cases over a server-owned Store. Closing
// the returned value does not close the shared database.
func FromStore(database *store.Store) *Application {
	return newApplication(database)
}

// FromStoreWithRuntimeResolver exposes application use cases over a
// server-owned Store while replacing only live runtime identity resolution.
// Production composition should normally use FromStore or Open.
func FromStoreWithRuntimeResolver(database *store.Store, resolver RuntimeResolver) *Application {
	application := newApplication(database)
	application.runtime = resolver
	return application
}

// FromStoreWithSettings exposes server-owned services that also require the
// configured data root or authenticated upstream credentials.
func FromStoreWithSettings(database *store.Store, configuration settings.Settings) *Application {
	application := newApplication(database)
	application.settings = configuration
	return application
}

func (application *Application) Close() error {
	if application == nil {
		return nil
	}
	if !application.ownsDatabase {
		return nil
	}
	return application.database.Close()
}

func (application *Application) CanonicalHead(ctx context.Context) (*CanonicalSnapshot, error) {
	head, err := application.database.Head(ctx)
	if err != nil {
		return nil, err
	}
	if head == nil {
		return nil, nil
	}
	value := snapshot(*head)
	return &value, nil
}

// ResolveCoreVersion follows the CLI contract exactly: an explicit version is
// validated as-is; omission resolves the currently running, OS-verified exact
// identity and never falls back to latest, desired, or previously edited data.
func (application *Application) ResolveCoreVersion(
	ctx context.Context,
	explicit string,
) (CoreVersionResolution, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		version, err := coreartifact.ParseExactVersion(explicit)
		if err != nil || version.IsZero() {
			return CoreVersionResolution{}, fmt.Errorf("invalid exact core version %q", explicit)
		}
		return CoreVersionResolution{ExactVersion: version.String(), Source: "explicit"}, nil
	}
	if application == nil || application.runtime == nil {
		return CoreVersionResolution{}, runtimeidentity.ErrInspectionUnavailable
	}
	running, err := application.runtime.Resolve(ctx)
	if err != nil {
		return CoreVersionResolution{}, err
	}
	return CoreVersionResolution{
		ExactVersion: running.ExactCoreVersion,
		Source:       "running",
		Running:      &running,
	}, nil
}

func (application *Application) ListCanonicalRevisions(
	ctx context.Context,
	beforeSequence int64,
	limit int,
) (CanonicalRevisionPage, error) {
	var cursor *store.CanonicalRevisionCursor
	if beforeSequence != 0 {
		cursor = &store.CanonicalRevisionCursor{BeforeSequence: beforeSequence}
	}
	page, err := application.database.ListCanonicalRevisions(ctx, store.CanonicalRevisionListFilter{
		Cursor: cursor,
		Limit:  limit,
	})
	if err != nil {
		return CanonicalRevisionPage{}, err
	}
	result := CanonicalRevisionPage{Items: make([]CanonicalSnapshot, len(page.Items))}
	for index, revision := range page.Items {
		result.Items[index] = snapshot(revision)
	}
	if page.Next != nil {
		next := page.Next.BeforeSequence
		result.Next = &next
	}
	return result, nil
}

// CanonicalRevision resolves either a stable revision ID or a #sequence
// reference. The explicit prefix keeps IDs and sequences unambiguous.
func (application *Application) CanonicalRevision(
	ctx context.Context,
	reference string,
) (CanonicalSnapshot, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return CanonicalSnapshot{}, errors.New("canonical revision reference is empty")
	}
	var (
		revision store.CanonicalRevision
		err      error
	)
	if strings.HasPrefix(reference, "#") {
		sequence, parseErr := strconv.ParseInt(strings.TrimPrefix(reference, "#"), 10, 64)
		if parseErr != nil || sequence < 1 {
			return CanonicalSnapshot{}, fmt.Errorf("invalid canonical revision sequence %q", reference)
		}
		revision, err = application.database.GetCanonicalRevisionBySequence(ctx, sequence)
	} else {
		revision, err = application.database.GetCanonicalRevision(ctx, reference)
	}
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	return snapshot(revision), nil
}

func (application *Application) DiffCanonicalRevisions(
	ctx context.Context,
	fromReference string,
	toReference string,
) (CanonicalRevisionDiff, error) {
	from, err := application.CanonicalRevision(ctx, fromReference)
	if err != nil {
		return CanonicalRevisionDiff{}, err
	}
	to, err := application.CanonicalRevision(ctx, toReference)
	if err != nil {
		return CanonicalRevisionDiff{}, err
	}
	var fromValue, toValue any
	if err := decodeCanonicalValue(from.Document, &fromValue); err != nil {
		return CanonicalRevisionDiff{}, err
	}
	if err := decodeCanonicalValue(to.Document, &toValue); err != nil {
		return CanonicalRevisionDiff{}, err
	}
	changes := make([]CanonicalDiffEntry, 0)
	collectCanonicalDiff("", fromValue, true, toValue, true, &changes)
	return CanonicalRevisionDiff{From: from, To: to, Changes: changes}, nil
}

func (application *Application) RestoreCanonicalRevision(
	ctx context.Context,
	expectedHead string,
	targetReference string,
) (CanonicalSave, error) {
	target, err := application.CanonicalRevision(ctx, targetReference)
	if err != nil {
		return CanonicalSave{}, err
	}
	return application.ReplaceCanonical(ctx, expectedHead, target.Document)
}

func (application *Application) ListTasks(ctx context.Context, filter TaskListFilter) (TaskPage, error) {
	page, err := application.database.ListTasks(ctx, store.TaskListFilter{
		Lane:   filter.Lane,
		Status: filter.Status,
		Kind:   filter.Kind,
		Cursor: filter.Cursor,
		Limit:  filter.Limit,
	})
	if err != nil {
		return TaskPage{}, err
	}
	result := TaskPage{Items: make([]Task, len(page.Items))}
	for index, task := range page.Items {
		result.Items[index] = applicationTask(task)
	}
	if page.Next != nil {
		result.Next = &TaskCursor{CreatedAt: page.Next.CreatedAt, ID: page.Next.ID}
	}
	return result, nil
}

func (application *Application) Task(ctx context.Context, taskID string) (Task, error) {
	task, err := application.database.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	return applicationTask(task), nil
}

func (application *Application) CancelTask(ctx context.Context, taskID string) (Task, error) {
	task, err := application.database.RequestTaskCancellation(ctx, taskID, application.now().UTC())
	if err != nil {
		return Task{}, err
	}
	return applicationTask(task), nil
}

// ReplaceCanonical validates a complete full snapshot and advances the head
// only if expectedHead still matches. An identical document is a no-op, but
// only after the same compare-and-swap precondition has been checked.
func (application *Application) ReplaceCanonical(
	ctx context.Context,
	expectedHead string,
	raw []byte,
) (CanonicalSave, error) {
	document, err := canonical.Parse(raw)
	if err != nil {
		return CanonicalSave{}, err
	}
	return application.saveCanonicalDocument(ctx, expectedHead, document)
}

func (application *Application) CanonicalValueAt(
	ctx context.Context,
	pointer string,
) (CanonicalValue, error) {
	head, document, err := application.headDocument(ctx)
	if err != nil {
		return CanonicalValue{}, err
	}
	value, err := document.ValueAtPointer(pointer)
	if err != nil {
		return CanonicalValue{}, err
	}
	return CanonicalValue{Revision: snapshot(*head), Pointer: pointer, Value: value}, nil
}

func (application *Application) SetCanonicalValue(
	ctx context.Context,
	expectedHead string,
	pointer string,
	rawValue []byte,
) (CanonicalSave, error) {
	var value any
	if err := jsonstrict.Decode(rawValue, canonical.MaximumBytes, &value); err != nil {
		return CanonicalSave{}, fmt.Errorf("canonical pointer value: %w", err)
	}
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.SetPointer(pointer, value)
	})
}

func (application *Application) UnsetCanonicalValue(
	ctx context.Context,
	expectedHead string,
	pointer string,
) (CanonicalSave, error) {
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.UnsetPointer(pointer)
	})
}

// PatchCanonical applies one ordered, bounded set of lossless JSON-pointer
// changes and advances the canonical head once. Values cross browser/API
// boundaries as JSON text, so untouched or edited large numbers are never
// coerced through JavaScript's binary64 number representation.
func (application *Application) PatchCanonical(
	ctx context.Context,
	expectedHead string,
	changes []CanonicalChange,
) (CanonicalSave, error) {
	const maximumCanonicalChanges = 4096
	if len(changes) == 0 || len(changes) > maximumCanonicalChanges {
		return CanonicalSave{}, fmt.Errorf(
			"%w: changes must contain between 1 and %d entries",
			ErrCanonicalPatchInvalid,
			maximumCanonicalChanges,
		)
	}
	head, document, err := application.headDocument(ctx)
	if err != nil {
		return CanonicalSave{}, err
	}
	if head.ID != expectedHead {
		return CanonicalSave{}, &store.RevisionConflictError{ExpectedHead: expectedHead, ActualHead: head.ID}
	}
	seen := make(map[string]struct{}, len(changes))
	updated := document
	totalValueBytes := 0
	for index, change := range changes {
		if change.Path == "" || len(change.Path) > 4096 {
			return CanonicalSave{}, fmt.Errorf("%w: changes[%d].path is empty or too long", ErrCanonicalPatchInvalid, index)
		}
		if _, duplicate := seen[change.Path]; duplicate {
			return CanonicalSave{}, fmt.Errorf("%w: duplicate path %q", ErrCanonicalPatchInvalid, change.Path)
		}
		seen[change.Path] = struct{}{}
		switch change.Operation {
		case "set":
			if change.ValueJSON == "" {
				return CanonicalSave{}, fmt.Errorf("%w: changes[%d].value_json is required", ErrCanonicalPatchInvalid, index)
			}
			totalValueBytes += len(change.ValueJSON)
			if totalValueBytes > canonical.MaximumBytes {
				return CanonicalSave{}, fmt.Errorf("%w: change values exceed %d bytes", ErrCanonicalPatchInvalid, canonical.MaximumBytes)
			}
			var value any
			if err := jsonstrict.Decode([]byte(change.ValueJSON), canonical.MaximumBytes, &value); err != nil {
				return CanonicalSave{}, fmt.Errorf("%w: changes[%d].value_json: %v", ErrCanonicalPatchInvalid, index, err)
			}
			updated, err = updated.SetPointer(change.Path, value)
		case "unset":
			if change.ValueJSON != "" {
				return CanonicalSave{}, fmt.Errorf("%w: changes[%d].value_json is forbidden for unset", ErrCanonicalPatchInvalid, index)
			}
			updated, err = updated.UnsetPointer(change.Path)
		default:
			return CanonicalSave{}, fmt.Errorf("%w: changes[%d].op %q is invalid", ErrCanonicalPatchInvalid, index, change.Operation)
		}
		if err != nil {
			return CanonicalSave{}, fmt.Errorf("%w: changes[%d] path %q: %v", ErrCanonicalPatchInvalid, index, change.Path, err)
		}
	}
	return application.saveCanonicalDocument(ctx, expectedHead, updated)
}

func (application *Application) saveCanonicalDocument(
	ctx context.Context,
	expectedHead string,
	document *canonical.Document,
) (CanonicalSave, error) {
	head, err := application.database.Head(ctx)
	if err != nil {
		return CanonicalSave{}, err
	}
	actualHead := ""
	if head != nil {
		actualHead = head.ID
	}
	if actualHead != expectedHead {
		return CanonicalSave{}, &store.RevisionConflictError{ExpectedHead: expectedHead, ActualHead: actualHead}
	}
	canonicalBytes := document.CanonicalJSON()
	digest := sha256.Sum256(canonicalBytes)
	if head != nil && head.SHA256 == hex.EncodeToString(digest[:]) {
		return CanonicalSave{Revision: snapshot(*head), NoChange: true}, nil
	}

	revisionID, err := application.newID("rev")
	if err != nil {
		return CanonicalSave{}, err
	}
	commandID, err := application.newID("cmd")
	if err != nil {
		return CanonicalSave{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return CanonicalSave{}, err
	}
	createdAt := application.now().UTC()
	revision, err := application.database.SaveCanonicalRevisionAndTask(
		ctx,
		expectedHead,
		store.NewCanonicalRevision{
			ID:            revisionID,
			SchemaVersion: canonical.SchemaVersion,
			Document:      canonicalBytes,
			CommandID:     commandID,
			CreatedAt:     createdAt,
		},
		store.NewTask{
			ID:             taskID,
			IdempotencyKey: "canonical:" + expectedHead + ":" + hex.EncodeToString(digest[:]),
			Lane:           store.TaskLaneMaintenance,
			Kind:           "canonical-saved",
			Payload:        json.RawMessage(`{"revision_id":"` + revisionID + `"}`),
			CreatedAt:      createdAt,
		},
	)
	if err != nil {
		return CanonicalSave{}, err
	}
	return CanonicalSave{Revision: snapshot(revision), TaskID: taskID}, nil
}

type EntityList struct {
	Revision CanonicalSnapshot `json:"revision"`
	Entities []map[string]any  `json:"entities"`
}

func (application *Application) ListEntities(ctx context.Context, collection canonical.Collection) (EntityList, error) {
	head, document, err := application.headDocument(ctx)
	if err != nil {
		return EntityList{}, err
	}
	entities, err := document.Entities(collection)
	if err != nil {
		return EntityList{}, err
	}
	return EntityList{Revision: snapshot(*head), Entities: entities}, nil
}

func (application *Application) GetEntity(ctx context.Context, collection canonical.Collection, identifier string) (CanonicalSnapshot, map[string]any, error) {
	head, document, err := application.headDocument(ctx)
	if err != nil {
		return CanonicalSnapshot{}, nil, err
	}
	entity, err := document.Entity(collection, identifier)
	if err != nil {
		return CanonicalSnapshot{}, nil, err
	}
	return snapshot(*head), entity, nil
}

func (application *Application) CreateEntity(
	ctx context.Context,
	expectedHead string,
	collection canonical.Collection,
	entity map[string]any,
) (CanonicalSave, error) {
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.CreateEntity(collection, entity)
	})
}

func (application *Application) ReplaceEntity(
	ctx context.Context,
	expectedHead string,
	collection canonical.Collection,
	identifier string,
	entity map[string]any,
) (CanonicalSave, error) {
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.ReplaceEntity(collection, identifier, entity)
	})
}

func (application *Application) DeleteEntity(
	ctx context.Context,
	expectedHead string,
	collection canonical.Collection,
	identifier string,
) (CanonicalSave, error) {
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.DeleteEntity(collection, identifier)
	})
}

func (application *Application) SetEntityEnabled(
	ctx context.Context,
	expectedHead string,
	collection canonical.Collection,
	identifier string,
	enabled bool,
) (CanonicalSave, error) {
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.SetEntityEnabled(collection, identifier, enabled)
	})
}

func (application *Application) MoveEntity(
	ctx context.Context,
	expectedHead string,
	collection canonical.Collection,
	identifier string,
	beforeID string,
) (CanonicalSave, error) {
	return application.editCanonical(ctx, expectedHead, func(document *canonical.Document) (*canonical.Document, error) {
		return document.MoveEntity(collection, identifier, beforeID)
	})
}

func (application *Application) editCanonical(
	ctx context.Context,
	expectedHead string,
	edit func(*canonical.Document) (*canonical.Document, error),
) (CanonicalSave, error) {
	head, document, err := application.headDocument(ctx)
	if err != nil {
		return CanonicalSave{}, err
	}
	if head.ID != expectedHead {
		return CanonicalSave{}, &store.RevisionConflictError{ExpectedHead: expectedHead, ActualHead: head.ID}
	}
	edited, err := edit(document)
	if err != nil {
		return CanonicalSave{}, err
	}
	return application.saveCanonicalDocument(ctx, expectedHead, edited)
}

func (application *Application) headDocument(ctx context.Context) (*store.CanonicalRevision, *canonical.Document, error) {
	head, err := application.database.Head(ctx)
	if err != nil {
		return nil, nil, err
	}
	if head == nil {
		return nil, nil, errors.New("canonical configuration is not initialized")
	}
	document, err := canonical.Parse(head.Document)
	if err != nil {
		return nil, nil, fmt.Errorf("parse stored canonical revision %q: %w", head.ID, err)
	}
	return head, document, nil
}

func (application *Application) newID(prefix string) (string, error) {
	raw := make([]byte, 16)
	n, err := application.random(raw)
	if err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	if n != len(raw) {
		return "", fmt.Errorf("generate %s id: short random read", prefix)
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}

func snapshot(value store.CanonicalRevision) CanonicalSnapshot {
	return CanonicalSnapshot{
		ID:            value.ID,
		Sequence:      value.Sequence,
		ParentID:      value.ParentID,
		SchemaVersion: value.SchemaVersion,
		Document:      append(json.RawMessage(nil), value.Document...),
		DocumentJSON:  string(value.Document),
		SHA256:        value.SHA256,
		CreatedAt:     value.CreatedAt,
	}
}

func applicationTask(value store.Task) Task {
	return Task{
		ID:                  value.ID,
		IdempotencyKey:      value.IdempotencyKey,
		Lane:                value.Lane,
		Kind:                value.Kind,
		Status:              value.Status,
		Generation:          value.Generation,
		CanonicalRevisionID: value.CanonicalRevisionID,
		StartupArtifactID:   value.StartupArtifactID,
		ActivationBundleID:  value.ActivationBundleID,
		Payload:             append(json.RawMessage(nil), value.Payload...),
		Result:              append(json.RawMessage(nil), value.Result...),
		Failure:             append(json.RawMessage(nil), value.Failure...),
		CancelRequested:     value.CancelRequested,
		Attempt:             value.Attempt,
		LeaseExpiresAt:      cloneTime(value.LeaseExpiresAt),
		NotBefore:           cloneTime(value.NotBefore),
		CreatedAt:           value.CreatedAt,
		UpdatedAt:           value.UpdatedAt,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func decodeCanonicalValue(raw json.RawMessage, target *any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode stored canonical revision: %w", err)
	}
	return nil
}

func collectCanonicalDiff(
	path string,
	from any,
	fromPresent bool,
	to any,
	toPresent bool,
	changes *[]CanonicalDiffEntry,
) {
	if !fromPresent || !toPresent {
		*changes = append(*changes, CanonicalDiffEntry{
			Path: displayPointer(path),
			From: DiffValue{Present: fromPresent, Value: from},
			To:   DiffValue{Present: toPresent, Value: to},
		})
		return
	}
	fromObject, fromIsObject := from.(map[string]any)
	toObject, toIsObject := to.(map[string]any)
	if fromIsObject && toIsObject {
		keys := make([]string, 0, len(fromObject)+len(toObject))
		seen := make(map[string]struct{}, len(fromObject)+len(toObject))
		for key := range fromObject {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		for key := range toObject {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			fromChild, fromOK := fromObject[key]
			toChild, toOK := toObject[key]
			collectCanonicalDiff(path+"/"+escapePointerToken(key), fromChild, fromOK, toChild, toOK, changes)
		}
		return
	}
	// Ordered collections are semantic. Treat each array as one atomic value so
	// moves remain readable instead of producing misleading index edits.
	if reflect.DeepEqual(from, to) {
		return
	}
	*changes = append(*changes, CanonicalDiffEntry{
		Path: displayPointer(path),
		From: DiffValue{Present: true, Value: from},
		To:   DiffValue{Present: true, Value: to},
	})
}

func escapePointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func displayPointer(value string) string {
	if value == "" {
		return "/"
	}
	return value
}

func IsRevisionConflict(err error) bool {
	return errors.Is(err, store.ErrRevisionConflict)
}

func IsRevisionNotFound(err error) bool {
	return errors.Is(err, store.ErrCanonicalRevisionNotFound)
}

func IsTaskNotFound(err error) bool {
	return errors.Is(err, store.ErrTaskNotFound)
}
