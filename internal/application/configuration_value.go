// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) ReplaceConfiguration(
	ctx context.Context,
	expectedHead string,
	raw []byte,
) (CanonicalSave, error) {
	document, err := configuration.ParseV2(raw)
	if err != nil {
		return CanonicalSave{}, err
	}
	return application.saveCanonicalV2Document(ctx, expectedHead, document)
}

func (application *Application) PatchConfiguration(
	ctx context.Context,
	expectedHead string,
	changes []CanonicalChange,
) (CanonicalSave, error) {
	if len(changes) == 0 || len(changes) > 4096 {
		return CanonicalSave{}, ErrCanonicalPatchInvalid
	}
	head, document, err := application.configurationHeadDocument(ctx)
	if err != nil {
		return CanonicalSave{}, err
	}
	if head.ID != expectedHead {
		return CanonicalSave{}, &store.RevisionConflictError{ExpectedHead: expectedHead, ActualHead: head.ID}
	}
	updated := document
	seen := make(map[string]struct{}, len(changes))
	for index, change := range changes {
		if change.Path == "" {
			return CanonicalSave{}, fmt.Errorf("%w: changes[%d].path is empty", ErrCanonicalPatchInvalid, index)
		}
		if _, duplicate := seen[change.Path]; duplicate {
			return CanonicalSave{}, fmt.Errorf("%w: duplicate path %q", ErrCanonicalPatchInvalid, change.Path)
		}
		seen[change.Path] = struct{}{}
		switch change.Operation {
		case "set":
			var value any
			if change.ValueJSON == "" || jsonstrict.Decode([]byte(change.ValueJSON), configuration.MaximumBytes, &value) != nil {
				return CanonicalSave{}, fmt.Errorf("%w: changes[%d].value_json is invalid", ErrCanonicalPatchInvalid, index)
			}
			updated, err = updated.SetPointer(change.Path, value)
		case "unset":
			if change.ValueJSON != "" {
				return CanonicalSave{}, fmt.Errorf("%w: changes[%d].value_json is forbidden", ErrCanonicalPatchInvalid, index)
			}
			updated, err = updated.UnsetPointer(change.Path)
		default:
			return CanonicalSave{}, fmt.Errorf("%w: changes[%d].op is invalid", ErrCanonicalPatchInvalid, index)
		}
		if err != nil {
			return CanonicalSave{}, fmt.Errorf("%w: changes[%d]: %v", ErrCanonicalPatchInvalid, index, err)
		}
	}
	return application.saveCanonicalV2Document(ctx, expectedHead, updated)
}

func (application *Application) configurationHeadDocument(
	ctx context.Context,
) (*store.CanonicalRevision, *configuration.V2Document, error) {
	head, err := application.database.Head(ctx)
	if err != nil {
		return nil, nil, err
	}
	if head == nil {
		return nil, nil, errors.New("canonical configuration is not initialized")
	}
	document, err := configuration.ParseV2(head.Document)
	if err != nil {
		return nil, nil, fmt.Errorf("parse stored canonical revision %q: %w", head.ID, err)
	}
	return head, document, nil
}
