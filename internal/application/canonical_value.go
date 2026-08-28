// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrCanonicalPatchInvalid = errors.New("canonical patch is invalid")

func (application *Application) ReplaceCanonical(
	ctx context.Context,
	expectedHead string,
	raw []byte,
) (CanonicalSave, error) {
	return application.ReplaceConfiguration(ctx, expectedHead, raw)
}

func (application *Application) CanonicalValueAt(
	ctx context.Context,
	pointer string,
) (CanonicalValue, error) {
	head, document, err := application.configurationHeadDocument(ctx)
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
	return application.editConfiguration(ctx, expectedHead, func(document *canonical.V2Document) (*canonical.V2Document, error) {
		return document.SetPointer(pointer, value)
	})
}

func (application *Application) UnsetCanonicalValue(
	ctx context.Context,
	expectedHead string,
	pointer string,
) (CanonicalSave, error) {
	return application.editConfiguration(ctx, expectedHead, func(document *canonical.V2Document) (*canonical.V2Document, error) {
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
	return application.PatchConfiguration(ctx, expectedHead, changes)
}

func (application *Application) editConfiguration(
	ctx context.Context,
	expectedHead string,
	edit func(*canonical.V2Document) (*canonical.V2Document, error),
) (CanonicalSave, error) {
	head, document, err := application.configurationHeadDocument(ctx)
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
	return application.saveCanonicalV2Document(ctx, expectedHead, edited)
}

func (application *Application) saveCanonicalV2Document(
	ctx context.Context,
	expectedHead string,
	document *canonical.V2Document,
) (CanonicalSave, error) {
	return application.saveCanonicalBytes(ctx, expectedHead, canonical.SchemaVersionV2, document.CanonicalJSON())
}

func (application *Application) saveCanonicalBytes(
	ctx context.Context,
	expectedHead string,
	schemaVersion int,
	canonicalBytes []byte,
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
			SchemaVersion: schemaVersion,
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
