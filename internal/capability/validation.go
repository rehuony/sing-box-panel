// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaximumSemanticFacts  = 4096
	MaximumTransforms     = 4096
	MaximumUIDescriptors  = 4096
	MaximumOwnedPaths     = 64
	MaximumTransformPaths = 64
	MaximumEnumEntries    = 1024
	MaximumUIOptions      = 1024
	maximumIdentifier     = 128
	maximumLabelBytes     = 512
	maximumHelpBytes      = 16 << 10
)

func validateManifestSpec(spec ManifestSpec) error {
	if spec.SchemaVersion != ManifestSchemaVersion {
		return manifestError("schema_version must be %d", ManifestSchemaVersion)
	}
	if spec.CoreVersion.IsZero() {
		return manifestError("core_version must not be 0.0.0")
	}
	if !validSupportLevel(spec.SupportLevel) {
		return manifestError("unknown support_level %q", spec.SupportLevel)
	}
	if len(spec.SemanticFacts) > MaximumSemanticFacts {
		return manifestError("semantic_facts exceeds %d entries", MaximumSemanticFacts)
	}
	if len(spec.Transforms) > MaximumTransforms {
		return manifestError("transforms exceeds %d entries", MaximumTransforms)
	}
	if len(spec.UI) > MaximumUIDescriptors {
		return manifestError("ui exceeds %d entries", MaximumUIDescriptors)
	}
	if spec.SupportLevel == SupportManualJSON || spec.SupportLevel == SupportUnavailable {
		if len(spec.SemanticFacts) != 0 || len(spec.Transforms) != 0 || len(spec.UI) != 0 {
			return manifestError("%s manifest cannot contain structured facts, transforms, or UI", spec.SupportLevel)
		}
		return validateManifestSize(spec)
	}
	if len(spec.SemanticFacts) == 0 {
		return manifestError("structured manifest requires at least one semantic fact")
	}

	facts := make(map[string]SemanticFact, len(spec.SemanticFacts))
	canonicalOwners := make(map[string]string)
	versionOwners := make(map[string]string)
	for index, fact := range spec.SemanticFacts {
		if !validIdentifier(fact.ID) {
			return manifestError("semantic_facts[%d].id %q is invalid", index, fact.ID)
		}
		if _, duplicate := facts[fact.ID]; duplicate {
			return manifestError("duplicate semantic fact %q", fact.ID)
		}
		if _, err := parsePointer(fact.CanonicalPath); err != nil {
			return manifestError("semantic fact %q canonical_path: %v", fact.ID, err)
		}
		if !validClassification(fact.Classification) {
			return manifestError("semantic fact %q has unknown classification %q", fact.ID, fact.Classification)
		}
		if len(fact.OwnedPaths) > MaximumOwnedPaths {
			return manifestError("semantic fact %q owned_paths exceeds %d entries", fact.ID, MaximumOwnedPaths)
		}
		if fact.Classification == CoverageIntentionallyUnsupported && len(fact.OwnedPaths) != 0 {
			return manifestError("intentionally unsupported fact %q cannot own version paths", fact.ID)
		}
		if fact.Classification != CoverageIntentionallyUnsupported && len(fact.OwnedPaths) == 0 {
			return manifestError("fact %q requires at least one owned version path", fact.ID)
		}
		if conflictOwner, conflictPath := overlappingOwner(canonicalOwners, fact.CanonicalPath); conflictOwner != "" {
			return manifestError(
				"canonical ownership overlap: fact %q path %q conflicts with fact %q path %q",
				fact.ID, fact.CanonicalPath, conflictOwner, conflictPath,
			)
		}
		canonicalOwners[fact.CanonicalPath] = fact.ID
		for ownedIndex, ownedPath := range fact.OwnedPaths {
			if _, err := parsePointer(ownedPath); err != nil {
				return manifestError("semantic fact %q owned_paths[%d]: %v", fact.ID, ownedIndex, err)
			}
			if conflictOwner, conflictPath := overlappingOwner(versionOwners, ownedPath); conflictOwner != "" {
				return manifestError(
					"version ownership overlap: fact %q path %q conflicts with fact %q path %q",
					fact.ID, ownedPath, conflictOwner, conflictPath,
				)
			}
			versionOwners[ownedPath] = fact.ID
		}
		facts[fact.ID] = fact
	}

	transformCounts := make(map[string]int, len(facts))
	transformIDs := make(map[string]struct{}, len(spec.Transforms))
	canonicalWrites := make(map[string]string)
	versionWrites := make(map[string]string)
	for index, transform := range spec.Transforms {
		if !validIdentifier(transform.ID) {
			return manifestError("transforms[%d].id %q is invalid", index, transform.ID)
		}
		if _, duplicate := transformIDs[transform.ID]; duplicate {
			return manifestError("duplicate transform %q", transform.ID)
		}
		transformIDs[transform.ID] = struct{}{}
		fact, exists := facts[transform.FactID]
		if !exists {
			return manifestError("transform %q references unclassified fact %q", transform.ID, transform.FactID)
		}
		if fact.Classification == CoverageIntentionallyUnsupported {
			return manifestError("transform %q references intentionally unsupported fact %q", transform.ID, transform.FactID)
		}
		if err := validateTransform(transform); err != nil {
			return manifestError("transform %q: %v", transform.ID, err)
		}
		for _, path := range transform.From {
			if !pointerContains(fact.CanonicalPath, path) {
				return manifestError("transform %q canonical path %q is outside fact %q ownership", transform.ID, path, fact.ID)
			}
			if conflict, conflictPath := overlappingOwner(canonicalWrites, path); conflict != "" {
				return manifestError("transform %q canonical write %q overlaps transform %q path %q", transform.ID, path, conflict, conflictPath)
			}
			canonicalWrites[path] = transform.ID
		}
		for _, path := range transform.To {
			if !ownedByFact(path, fact.OwnedPaths) {
				return manifestError("transform %q version path %q is outside fact %q ownership", transform.ID, path, fact.ID)
			}
			if conflict, conflictPath := overlappingOwner(versionWrites, path); conflict != "" {
				return manifestError("transform %q version write %q overlaps transform %q path %q", transform.ID, path, conflict, conflictPath)
			}
			versionWrites[path] = transform.ID
		}
		transformCounts[transform.FactID]++
	}
	for _, fact := range spec.SemanticFacts {
		if fact.Classification != CoverageIntentionallyUnsupported && transformCounts[fact.ID] == 0 {
			return manifestError("classified fact %q has no transform", fact.ID)
		}
	}
	for _, transform := range spec.Transforms {
		if transform.Primitive != PrimitiveConditional {
			continue
		}
		if !pathCoveredByWrites(canonicalWrites, transform.When.CanonicalPath) {
			return manifestError("transform %q condition canonical_path %q is not projected by any transform", transform.ID, transform.When.CanonicalPath)
		}
		if !pathCoveredByWrites(versionWrites, transform.When.VersionPath) {
			return manifestError("transform %q condition version_path %q is not projected by any transform", transform.ID, transform.When.VersionPath)
		}
	}

	uiIDs := make(map[string]struct{}, len(spec.UI))
	for index, descriptor := range spec.UI {
		if err := validateUIDescriptor(descriptor, facts); err != nil {
			return manifestError("ui[%d]: %v", index, err)
		}
		if _, duplicate := uiIDs[descriptor.ID]; duplicate {
			return manifestError("duplicate UI descriptor %q", descriptor.ID)
		}
		if descriptor.VisibleWhen != nil && !ownedByCanonicalFact(descriptor.VisibleWhen.CanonicalPath, spec.SemanticFacts) {
			return manifestError("UI descriptor %q visibility path %q is not owned by a semantic fact", descriptor.ID, descriptor.VisibleWhen.CanonicalPath)
		}
		uiIDs[descriptor.ID] = struct{}{}
	}
	return validateManifestSize(spec)
}

func validateTransform(transform Transform) error {
	if len(transform.From) > MaximumTransformPaths || len(transform.To) > MaximumTransformPaths {
		return fmt.Errorf("from or to exceeds %d paths", MaximumTransformPaths)
	}
	for index, path := range transform.From {
		if _, err := parsePointer(path); err != nil {
			return fmt.Errorf("from[%d]: %v", index, err)
		}
	}
	for index, path := range transform.To {
		if _, err := parsePointer(path); err != nil {
			return fmt.Errorf("to[%d]: %v", index, err)
		}
	}
	if len(transform.From) == 0 || len(transform.To) == 0 {
		return fmt.Errorf("from and to must not be empty")
	}

	simple := func() error {
		if len(transform.From) != 1 || len(transform.To) != 1 {
			return fmt.Errorf("%s requires exactly one from and one to path", transform.Primitive)
		}
		return nil
	}
	noOptions := func() error {
		if transform.Separator != "" || transform.Key != "" || len(transform.Enum) != 0 || transform.When != nil {
			return fmt.Errorf("%s contains options belonging to another primitive", transform.Primitive)
		}
		return nil
	}

	switch transform.Primitive {
	case PrimitiveRename, PrimitivePresence:
		if err := simple(); err != nil {
			return err
		}
		return noOptions()
	case PrimitiveWrap, PrimitiveUnwrap:
		if err := simple(); err != nil {
			return err
		}
		if !validObjectKey(transform.Key) {
			return fmt.Errorf("%s requires a safe object key", transform.Primitive)
		}
		if transform.Separator != "" || len(transform.Enum) != 0 || transform.When != nil {
			return fmt.Errorf("%s contains options belonging to another primitive", transform.Primitive)
		}
	case PrimitiveSplit:
		if len(transform.From) != 1 || len(transform.To) < 2 {
			return fmt.Errorf("split requires one from path and at least two to paths")
		}
		if !validSeparator(transform.Separator) || transform.Key != "" || len(transform.Enum) != 0 || transform.When != nil {
			return fmt.Errorf("split requires only a non-empty separator")
		}
	case PrimitiveJoin:
		if len(transform.From) < 2 || len(transform.To) != 1 {
			return fmt.Errorf("join requires at least two from paths and one to path")
		}
		if !validSeparator(transform.Separator) || transform.Key != "" || len(transform.Enum) != 0 || transform.When != nil {
			return fmt.Errorf("join requires only a non-empty separator")
		}
	case PrimitiveEnum:
		if err := simple(); err != nil {
			return err
		}
		if transform.Separator != "" || transform.Key != "" || transform.When != nil || len(transform.Enum) == 0 {
			return fmt.Errorf("enum requires only a non-empty mapping")
		}
		if len(transform.Enum) > MaximumEnumEntries {
			return fmt.Errorf("enum exceeds %d entries", MaximumEnumEntries)
		}
		reverse := make(map[string]string, len(transform.Enum))
		for source, target := range transform.Enum {
			if source == "" || target == "" {
				return fmt.Errorf("enum values cannot be empty")
			}
			if previous, duplicate := reverse[target]; duplicate {
				return fmt.Errorf("enum mapping is not reversible: %q and %q both map to %q", previous, source, target)
			}
			reverse[target] = source
		}
	case PrimitiveConditional:
		if err := simple(); err != nil {
			return err
		}
		if transform.Separator != "" || transform.Key != "" || len(transform.Enum) != 0 || transform.When == nil {
			return fmt.Errorf("conditional requires only a when predicate")
		}
		if _, err := parsePointer(transform.When.CanonicalPath); err != nil {
			return fmt.Errorf("when canonical_path: %v", err)
		}
		if _, err := parsePointer(transform.When.VersionPath); err != nil {
			return fmt.Errorf("when version_path: %v", err)
		}
		if !validScalar(transform.When.Equals) {
			return fmt.Errorf("when equals must be a finite JSON scalar")
		}
	default:
		return fmt.Errorf("unknown primitive %q", transform.Primitive)
	}
	return nil
}

func validateUIDescriptor(descriptor UIDescriptor, facts map[string]SemanticFact) error {
	if !validIdentifier(descriptor.ID) {
		return fmt.Errorf("id %q is invalid", descriptor.ID)
	}
	switch descriptor.Kind {
	case UIGroup, UIText, UINumber, UIBoolean, UISelect, UIJSON:
	default:
		return fmt.Errorf("descriptor %q has unknown kind %q", descriptor.ID, descriptor.Kind)
	}
	if descriptor.Kind == UIGroup && descriptor.FactID == "" {
		// A layout group can span several semantic facts and therefore need not
		// claim one fact. Every value-bearing descriptor remains fact-bound.
	} else if _, exists := facts[descriptor.FactID]; !exists {
		return fmt.Errorf("descriptor %q references unclassified fact %q", descriptor.ID, descriptor.FactID)
	}
	if !validDisplayText(descriptor.Label, maximumLabelBytes, false) {
		return fmt.Errorf("descriptor %q label is empty, invalid, or too long", descriptor.ID)
	}
	if !validDisplayText(descriptor.Help, maximumHelpBytes, true) {
		return fmt.Errorf("descriptor %q help is invalid or too long", descriptor.ID)
	}
	if descriptor.Order < 0 || descriptor.Order > 1_000_000 {
		return fmt.Errorf("descriptor %q order is outside 0..1000000", descriptor.ID)
	}
	if descriptor.Kind == UISelect && len(descriptor.Options) == 0 {
		return fmt.Errorf("select descriptor %q requires options", descriptor.ID)
	}
	if len(descriptor.Options) > MaximumUIOptions {
		return fmt.Errorf("descriptor %q exceeds %d options", descriptor.ID, MaximumUIOptions)
	}
	if descriptor.Kind != UISelect && len(descriptor.Options) != 0 {
		return fmt.Errorf("only select descriptor %q can contain options", descriptor.ID)
	}
	values := make(map[string]struct{}, len(descriptor.Options))
	for _, option := range descriptor.Options {
		if option.Value == "" || !validDisplayText(option.Label, maximumLabelBytes, false) {
			return fmt.Errorf("descriptor %q has an invalid option", descriptor.ID)
		}
		if _, duplicate := values[option.Value]; duplicate {
			return fmt.Errorf("descriptor %q has duplicate option value %q", descriptor.ID, option.Value)
		}
		values[option.Value] = struct{}{}
	}
	if descriptor.VisibleWhen != nil {
		if _, err := parsePointer(descriptor.VisibleWhen.CanonicalPath); err != nil {
			return fmt.Errorf("descriptor %q visible_when: %v", descriptor.ID, err)
		}
		if !validScalar(descriptor.VisibleWhen.Equals) {
			return fmt.Errorf("descriptor %q visible_when equals must be a finite JSON scalar", descriptor.ID)
		}
	}
	return nil
}

func validSupportLevel(level SupportLevel) bool {
	switch level {
	case SupportNativeStructured, SupportCompatibleStructured, SupportManualJSON, SupportUnavailable:
		return true
	default:
		return false
	}
}

func validClassification(classification CoverageClassification) bool {
	switch classification {
	case CoverageSupported, CoverageIntentionallyUnsupported, CoverageBehaviorChanged:
		return true
	default:
		return false
	}
}

func validRepository(repository string) bool {
	if len(repository) == 0 || len(repository) > 200 || strings.TrimSpace(repository) != repository {
		return false
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		if part == "." || part == ".." {
			return false
		}
		for _, character := range part {
			if !(character >= 'a' && character <= 'z') &&
				!(character >= 'A' && character <= 'Z') &&
				!(character >= '0' && character <= '9') &&
				character != '-' && character != '_' && character != '.' {
				return false
			}
		}
	}
	return true
}

func validCommit(commit string) bool {
	if len(commit) != 40 && len(commit) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(commit)
	if err != nil || strings.ToLower(commit) != commit {
		return false
	}
	for _, value := range decoded {
		if value != 0 {
			return true
		}
	}
	return false
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > maximumIdentifier || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validObjectKey(value string) bool {
	if value == "" || len(value) > maximumIdentifier || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSeparator(value string) bool {
	return value != "" && len(value) <= 32 && utf8.ValidString(value)
}

func validDisplayText(value string, maximumBytes int, emptyAllowed bool) bool {
	if (!emptyAllowed && value == "") || len(value) > maximumBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == 0 || (unicode.IsControl(character) && character != '\n' && character != '\t') {
			return false
		}
	}
	return true
}

func validScalar(value any) bool {
	switch number := value.(type) {
	case nil, bool, string:
		return true
	case json.Number:
		_, valid := canonicalJSONNumber(number.String())
		return valid
	case float32:
		return !math.IsInf(float64(number), 0) && !math.IsNaN(float64(number))
	case float64:
		return !math.IsInf(number, 0) && !math.IsNaN(number)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func canonicalScalarJSON(value any) ([]byte, error) {
	if number, ok := value.(json.Number); ok {
		canonical, valid := canonicalJSONNumber(number.String())
		if !valid {
			return nil, fmt.Errorf("invalid JSON number %q", number)
		}
		return []byte(canonical), nil
	}
	if !validScalar(value) {
		return nil, fmt.Errorf("value of type %T is not a finite JSON scalar", value)
	}
	return json.Marshal(value)
}

func normalizedScalar(value any) any {
	if number, ok := value.(json.Number); ok {
		canonical, valid := canonicalJSONNumber(number.String())
		if valid {
			return json.Number(canonical)
		}
	}
	return value
}

func canonicalJSONNumber(value string) (string, bool) {
	if _, err := json.Marshal(json.Number(value)); err != nil {
		return "", false
	}
	if !strings.ContainsAny(value, ".eE") {
		integer := new(big.Int)
		if _, valid := integer.SetString(value, 10); !valid {
			return "", false
		}
		return integer.String(), true
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return "", false
	}
	encoded, err := json.Marshal(number)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func ownedByFact(path string, ownedPaths []string) bool {
	for _, owner := range ownedPaths {
		if pointerContains(owner, path) {
			return true
		}
	}
	return false
}

func overlappingOwner(owners map[string]string, candidate string) (owner, path string) {
	for existingPath, existingOwner := range owners {
		if pointersOverlap(existingPath, candidate) {
			return existingOwner, existingPath
		}
	}
	return "", ""
}

func pathCoveredByWrites(writes map[string]string, candidate string) bool {
	for path := range writes {
		if pointerContains(path, candidate) {
			return true
		}
	}
	return false
}

func ownedByCanonicalFact(candidate string, facts []SemanticFact) bool {
	for _, fact := range facts {
		if pointerContains(fact.CanonicalPath, candidate) {
			return true
		}
	}
	return false
}

func manifestError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidManifest, fmt.Sprintf(format, arguments...))
}

func validateManifestSize(spec ManifestSpec) error {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return manifestError("encode validated manifest: %v", err)
	}
	if len(encoded) > MaximumManifestBytes {
		return manifestError("manifest exceeds %d bytes", MaximumManifestBytes)
	}
	return nil
}
