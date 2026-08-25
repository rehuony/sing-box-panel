// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type DiagnosticSeverity string

const (
	DiagnosticWarning DiagnosticSeverity = "warning"
)

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code"`
	FactID   string             `json:"fact_id"`
	Message  string             `json:"message"`
}

type ProjectionResult struct {
	Document    map[string]any `json:"document"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
}

// Projector executes only the fixed primitives represented by a validated
// manifest. It owns a defensive manifest copy and has no hooks for loading or
// invoking external code.
type Projector struct {
	manifest *Manifest
}

func NewProjector(manifest *Manifest) (*Projector, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	stable, err := NewManifest(manifest.Spec())
	if err != nil {
		return nil, err
	}
	if stable.SupportLevel() != SupportNativeStructured && stable.SupportLevel() != SupportCompatibleStructured {
		return nil, fmt.Errorf("%w: support level %q has no structured projector", ErrProjection, stable.SupportLevel())
	}
	return &Projector{manifest: stable}, nil
}

func (projector *Projector) Project(canonical map[string]any) (ProjectionResult, error) {
	if projector == nil || projector.manifest == nil {
		return ProjectionResult{}, fmt.Errorf("%w: projector is nil", ErrProjection)
	}
	if canonical == nil {
		canonical = map[string]any{}
	}
	versionDocument := make(map[string]any)
	for _, transform := range projector.manifest.spec.Transforms {
		if err := applyForward(transform, canonical, versionDocument); err != nil {
			return ProjectionResult{}, fmt.Errorf("%w: transform %q: %v", ErrProjection, transform.ID, err)
		}
	}

	diagnostics := make([]Diagnostic, 0)
	for _, fact := range projector.manifest.spec.SemanticFacts {
		_, present, err := getPointer(canonical, fact.CanonicalPath)
		if err != nil {
			return ProjectionResult{}, fmt.Errorf("%w: fact %q: %v", ErrProjection, fact.ID, err)
		}
		if !present {
			continue
		}
		switch fact.Classification {
		case CoverageIntentionallyUnsupported:
			diagnostics = append(diagnostics, Diagnostic{
				Severity: DiagnosticWarning,
				Code:     "fact_omitted",
				FactID:   fact.ID,
				Message:  "semantic fact is intentionally unsupported by this exact core version",
			})
		case CoverageBehaviorChanged:
			diagnostics = append(diagnostics, Diagnostic{
				Severity: DiagnosticWarning,
				Code:     "behavior_changed",
				FactID:   fact.ID,
				Message:  "semantic fact has declared behavior differences in this exact core version",
			})
		}
	}
	return ProjectionResult{Document: versionDocument, Diagnostics: diagnostics}, nil
}

func (projector *Projector) Reverse(versionDocument map[string]any) (map[string]any, error) {
	if projector == nil || projector.manifest == nil {
		return nil, fmt.Errorf("%w: projector is nil", ErrProjection)
	}
	if versionDocument == nil {
		versionDocument = map[string]any{}
	}
	if err := projector.validateVersionDocumentCoverage(versionDocument); err != nil {
		return nil, err
	}
	canonical := make(map[string]any)
	for index := len(projector.manifest.spec.Transforms) - 1; index >= 0; index-- {
		transform := projector.manifest.spec.Transforms[index]
		if err := applyReverse(transform, versionDocument, canonical); err != nil {
			return nil, fmt.Errorf("%w: transform %q: %v", ErrProjection, transform.ID, err)
		}
	}
	return canonical, nil
}

func (projector *Projector) validateVersionDocumentCoverage(versionDocument map[string]any) error {
	leaves := make([]string, 0)
	values := 0
	if err := collectLeafPointers(versionDocument, nil, 0, &values, &leaves); err != nil {
		return fmt.Errorf("%w: inspect version document: %v", ErrProjection, err)
	}
	targets := make([]string, 0)
	for _, transform := range projector.manifest.spec.Transforms {
		targets = append(targets, transform.To...)
	}
	for _, leaf := range leaves {
		covered := false
		for _, target := range targets {
			if pointerContains(target, leaf) {
				covered = true
				break
			}
		}
		if !covered {
			return fmt.Errorf("%w: version path %q is not owned by a transform", ErrProjection, leaf)
		}
	}
	return nil
}

func collectLeafPointers(value any, tokens []string, depth int, values *int, leaves *[]string) error {
	if depth > maximumJSONDepth {
		return fmt.Errorf("document nesting exceeds depth %d", maximumJSONDepth)
	}
	*values++
	if *values > maximumJSONValues {
		return fmt.Errorf("document exceeds %d values", maximumJSONValues)
	}
	switch value := value.(type) {
	case map[string]any:
		if len(value) == 0 && len(tokens) > 0 {
			*leaves = append(*leaves, encodePointer(tokens))
			return nil
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := collectLeafPointers(value[key], appendPointerToken(tokens, key), depth+1, values, leaves); err != nil {
				return err
			}
		}
	case []any:
		if len(value) == 0 && len(tokens) > 0 {
			*leaves = append(*leaves, encodePointer(tokens))
			return nil
		}
		for index, item := range value {
			if err := collectLeafPointers(item, appendPointerToken(tokens, fmt.Sprintf("%d", index)), depth+1, values, leaves); err != nil {
				return err
			}
		}
	default:
		if _, err := cloneJSONValue(value); err != nil {
			return err
		}
		if len(tokens) == 0 {
			return fmt.Errorf("version document root is not an object")
		}
		*leaves = append(*leaves, encodePointer(tokens))
	}
	return nil
}

func appendPointerToken(tokens []string, token string) []string {
	result := make([]string, len(tokens)+1)
	copy(result, tokens)
	result[len(tokens)] = token
	return result
}

func applyForward(transform Transform, canonical, versionDocument map[string]any) error {
	switch transform.Primitive {
	case PrimitiveRename, PrimitivePresence:
		return copyIfPresent(canonical, transform.From[0], versionDocument, transform.To[0])
	case PrimitiveWrap:
		value, present, err := getPointer(canonical, transform.From[0])
		if err != nil || !present {
			return err
		}
		return setPointer(versionDocument, transform.To[0], map[string]any{transform.Key: value})
	case PrimitiveUnwrap:
		value, present, err := getPointer(canonical, transform.From[0])
		if err != nil || !present {
			return err
		}
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("unwrap source %q is %T, not an object", transform.From[0], value)
		}
		if len(object) != 1 {
			return fmt.Errorf("unwrap source %q must contain only key %q", transform.From[0], transform.Key)
		}
		unwrapped, exists := object[transform.Key]
		if !exists {
			return fmt.Errorf("unwrap source %q does not contain key %q", transform.From[0], transform.Key)
		}
		return setPointer(versionDocument, transform.To[0], unwrapped)
	case PrimitiveSplit:
		value, present, err := getPointer(canonical, transform.From[0])
		if err != nil || !present {
			return err
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("split source %q is %T, not a string", transform.From[0], value)
		}
		parts := strings.Split(text, transform.Separator)
		if len(parts) != len(transform.To) {
			return fmt.Errorf("split source produced %d parts, expected %d", len(parts), len(transform.To))
		}
		for index, part := range parts {
			if err := setPointer(versionDocument, transform.To[index], part); err != nil {
				return err
			}
		}
		return nil
	case PrimitiveJoin:
		parts, present, err := collectStrings(canonical, transform.From)
		if err != nil || !present {
			return err
		}
		if err := rejectSeparatorInParts(parts, transform.Separator); err != nil {
			return err
		}
		return setPointer(versionDocument, transform.To[0], strings.Join(parts, transform.Separator))
	case PrimitiveEnum:
		value, present, err := getPointer(canonical, transform.From[0])
		if err != nil || !present {
			return err
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("enum source %q is %T, not a string", transform.From[0], value)
		}
		mapped, exists := transform.Enum[text]
		if !exists {
			return fmt.Errorf("enum source value %q is not mapped", text)
		}
		return setPointer(versionDocument, transform.To[0], mapped)
	case PrimitiveConditional:
		matches, err := conditionMatches(canonical, transform.When.CanonicalPath, transform.When.Equals)
		if err != nil || !matches {
			return err
		}
		return copyIfPresent(canonical, transform.From[0], versionDocument, transform.To[0])
	default:
		return fmt.Errorf("unknown primitive %q", transform.Primitive)
	}
}

func applyReverse(transform Transform, versionDocument, canonical map[string]any) error {
	switch transform.Primitive {
	case PrimitiveRename, PrimitivePresence:
		return copyIfPresent(versionDocument, transform.To[0], canonical, transform.From[0])
	case PrimitiveWrap:
		value, present, err := getPointer(versionDocument, transform.To[0])
		if err != nil || !present {
			return err
		}
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("wrapped source %q is %T, not an object", transform.To[0], value)
		}
		if len(object) != 1 {
			return fmt.Errorf("wrapped source %q must contain only key %q", transform.To[0], transform.Key)
		}
		unwrapped, exists := object[transform.Key]
		if !exists {
			return fmt.Errorf("wrapped source %q does not contain key %q", transform.To[0], transform.Key)
		}
		return setPointer(canonical, transform.From[0], unwrapped)
	case PrimitiveUnwrap:
		value, present, err := getPointer(versionDocument, transform.To[0])
		if err != nil || !present {
			return err
		}
		return setPointer(canonical, transform.From[0], map[string]any{transform.Key: value})
	case PrimitiveSplit:
		parts, present, err := collectStrings(versionDocument, transform.To)
		if err != nil || !present {
			return err
		}
		if err := rejectSeparatorInParts(parts, transform.Separator); err != nil {
			return err
		}
		return setPointer(canonical, transform.From[0], strings.Join(parts, transform.Separator))
	case PrimitiveJoin:
		value, present, err := getPointer(versionDocument, transform.To[0])
		if err != nil || !present {
			return err
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("joined source %q is %T, not a string", transform.To[0], value)
		}
		parts := strings.Split(text, transform.Separator)
		if len(parts) != len(transform.From) {
			return fmt.Errorf("joined source produced %d parts, expected %d", len(parts), len(transform.From))
		}
		for index, part := range parts {
			if err := setPointer(canonical, transform.From[index], part); err != nil {
				return err
			}
		}
		return nil
	case PrimitiveEnum:
		value, present, err := getPointer(versionDocument, transform.To[0])
		if err != nil || !present {
			return err
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("enum source %q is %T, not a string", transform.To[0], value)
		}
		for source, target := range transform.Enum {
			if target == text {
				return setPointer(canonical, transform.From[0], source)
			}
		}
		return fmt.Errorf("enum source value %q is not mapped", text)
	case PrimitiveConditional:
		matches, err := conditionMatches(versionDocument, transform.When.VersionPath, transform.When.Equals)
		if err != nil || !matches {
			return err
		}
		return copyIfPresent(versionDocument, transform.To[0], canonical, transform.From[0])
	default:
		return fmt.Errorf("unknown primitive %q", transform.Primitive)
	}
}

func copyIfPresent(source map[string]any, sourcePath string, target map[string]any, targetPath string) error {
	value, present, err := getPointer(source, sourcePath)
	if err != nil || !present {
		return err
	}
	return setPointer(target, targetPath, value)
}

func collectStrings(document map[string]any, paths []string) ([]string, bool, error) {
	values := make([]string, len(paths))
	presentCount := 0
	for index, path := range paths {
		value, present, err := getPointer(document, path)
		if err != nil {
			return nil, false, err
		}
		if !present {
			continue
		}
		presentCount++
		text, ok := value.(string)
		if !ok {
			return nil, false, fmt.Errorf("path %q is %T, not a string", path, value)
		}
		values[index] = text
	}
	if presentCount == 0 {
		return nil, false, nil
	}
	if presentCount != len(paths) {
		return nil, false, fmt.Errorf("only %d of %d joined paths are present", presentCount, len(paths))
	}
	return values, true, nil
}

func rejectSeparatorInParts(parts []string, separator string) error {
	for index, part := range parts {
		if strings.Contains(part, separator) {
			return fmt.Errorf("part %d contains separator %q and cannot round-trip", index, separator)
		}
	}
	return nil
}

func conditionMatches(document map[string]any, path string, expected any) (bool, error) {
	actual, present, err := getPointer(document, path)
	if err != nil || !present {
		return false, err
	}
	actualJSON, err := canonicalScalarJSON(actual)
	if err != nil {
		return false, fmt.Errorf("encode condition value at %q: %w", path, err)
	}
	expectedJSON, err := canonicalScalarJSON(expected)
	if err != nil {
		return false, fmt.Errorf("encode expected condition value: %w", err)
	}
	return string(actualJSON) == string(expectedJSON), nil
}

func getPointer(document map[string]any, pointer string) (any, bool, error) {
	tokens, err := parsePointer(pointer)
	if err != nil {
		return nil, false, err
	}
	var current any = document
	for _, token := range tokens {
		switch value := current.(type) {
		case map[string]any:
			next, exists := value[token]
			if !exists {
				return nil, false, nil
			}
			current = next
		case []any:
			index, valid := arrayIndex(token)
			if !valid || index >= len(value) {
				return nil, false, nil
			}
			current = value[index]
		default:
			return nil, false, fmt.Errorf("path %q traverses non-container %T at token %q", pointer, current, token)
		}
	}
	cloned, err := cloneJSONValue(current)
	if err != nil {
		return nil, false, err
	}
	return cloned, true, nil
}

func setPointer(document map[string]any, pointer string, value any) error {
	if document == nil {
		return fmt.Errorf("set path %q: destination document is nil", pointer)
	}
	tokens, err := parsePointer(pointer)
	if err != nil {
		return err
	}
	cloned, err := cloneJSONValue(value)
	if err != nil {
		return err
	}
	updated, err := setNode(document, tokens, cloned)
	if err != nil {
		return fmt.Errorf("set path %q: %w", pointer, err)
	}
	_, ok := updated.(map[string]any)
	if !ok {
		return fmt.Errorf("set path %q replaced document root with %T", pointer, updated)
	}
	return nil
}

func setNode(node any, tokens []string, value any) (any, error) {
	if len(tokens) == 0 {
		return value, nil
	}
	token := tokens[0]
	switch container := node.(type) {
	case map[string]any:
		child, exists := container[token]
		if !exists {
			child = nil
		}
		updated, err := setNode(child, tokens[1:], value)
		if err != nil {
			return nil, err
		}
		container[token] = updated
		return container, nil
	case []any:
		index, valid := arrayIndex(token)
		if !valid {
			return nil, fmt.Errorf("array token %q is not a canonical index", token)
		}
		if index > 100_000 {
			return nil, fmt.Errorf("array index %d exceeds safety limit", index)
		}
		for len(container) <= index {
			container = append(container, nil)
		}
		updated, err := setNode(container[index], tokens[1:], value)
		if err != nil {
			return nil, err
		}
		container[index] = updated
		return container, nil
	case nil:
		if _, numeric := arrayIndex(token); numeric {
			return setNode([]any{}, tokens, value)
		}
		return setNode(map[string]any{}, tokens, value)
	default:
		return nil, fmt.Errorf("cannot traverse %T at token %q", node, token)
	}
}

func cloneJSONValue(value any) (any, error) {
	switch value := value.(type) {
	case nil, bool, string, json.Number,
		float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		if !validScalar(value) {
			return nil, fmt.Errorf("non-finite JSON number")
		}
		return value, nil
	case map[string]any:
		clone := make(map[string]any, len(value))
		for key, item := range value {
			clonedItem, err := cloneJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("clone object key %q: %w", key, err)
			}
			clone[key] = clonedItem
		}
		return clone, nil
	case []any:
		clone := make([]any, len(value))
		for index, item := range value {
			clonedItem, err := cloneJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("clone array index %d: %w", index, err)
			}
			clone[index] = clonedItem
		}
		return clone, nil
	default:
		return nil, fmt.Errorf("value of type %T is not JSON data", value)
	}
}
