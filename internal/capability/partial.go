// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"fmt"
	"sort"
)

// ReversePartialResult contains only facts proven reversible by the pinned
// manifest. ResidualPaths remain owned exclusively by the exact manual
// artifact and are never inferred into canonical state.
type ReversePartialResult struct {
	Canonical     map[string]any `json:"canonical"`
	ResidualPaths []string       `json:"residual_paths,omitempty"`
}

// ReversePartial extracts manifest-owned transform targets, reverses that
// subset, and reports every unowned leaf without discarding it from the caller's
// original exact-byte document.
func (projector *Projector) ReversePartial(versionDocument map[string]any) (ReversePartialResult, error) {
	if projector == nil || projector.manifest == nil {
		return ReversePartialResult{}, fmt.Errorf("%w: projector is nil", ErrProjection)
	}
	if versionDocument == nil {
		versionDocument = map[string]any{}
	}
	targetSet := make(map[string]struct{})
	for _, transform := range projector.manifest.spec.Transforms {
		for _, target := range transform.To {
			targetSet[target] = struct{}{}
		}
		if transform.When != nil {
			targetSet[transform.When.VersionPath] = struct{}{}
		}
	}
	targets := make([]string, 0, len(targetSet))
	for target := range targetSet {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	owned := make(map[string]any)
	for _, target := range targets {
		value, present, err := getPointer(versionDocument, target)
		if err != nil {
			return ReversePartialResult{}, fmt.Errorf("%w: inspect owned path %q: %v", ErrProjection, target, err)
		}
		if !present {
			continue
		}
		if err := setPointer(owned, target, value); err != nil {
			return ReversePartialResult{}, fmt.Errorf("%w: copy owned path %q: %v", ErrProjection, target, err)
		}
	}
	canonical, err := projector.Reverse(owned)
	if err != nil {
		return ReversePartialResult{}, err
	}

	leaves := make([]string, 0)
	values := 0
	if err := collectLeafPointers(versionDocument, nil, 0, &values, &leaves); err != nil {
		return ReversePartialResult{}, fmt.Errorf("%w: inspect residual paths: %v", ErrProjection, err)
	}
	residual := make([]string, 0)
	for _, leaf := range leaves {
		covered := false
		for _, target := range targets {
			if pointerContains(target, leaf) {
				covered = true
				break
			}
		}
		if !covered {
			residual = append(residual, leaf)
		}
	}
	sort.Strings(residual)
	return ReversePartialResult{Canonical: canonical, ResidualPaths: residual}, nil
}
