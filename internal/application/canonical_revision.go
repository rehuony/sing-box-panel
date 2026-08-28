// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
)

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
