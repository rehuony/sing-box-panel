// SPDX-License-Identifier: GPL-3.0-or-later

// Package capability defines the declarative, exact-version contract used to
// project the panel's canonical configuration to and from sing-box JSON.
//
// The package intentionally has no executable plugin boundary. A manifest can
// only select one of the small, built-in transformation primitives declared
// here.
package capability

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

const ManifestSchemaVersion = 1

var (
	ErrInvalidReference = errors.New("invalid capability reference")
	ErrInvalidManifest  = errors.New("invalid capability manifest")
	ErrProjection       = errors.New("capability projection failed")
)

type SupportLevel string

const (
	SupportNativeStructured     SupportLevel = "native_structured"
	SupportCompatibleStructured SupportLevel = "compatible_structured"
	SupportManualJSON           SupportLevel = "manual_json"
	SupportUnavailable          SupportLevel = "unavailable"
)

type CoverageClassification string

const (
	CoverageSupported                CoverageClassification = "supported"
	CoverageIntentionallyUnsupported CoverageClassification = "intentionally_unsupported"
	CoverageBehaviorChanged          CoverageClassification = "behavior_changed"
)

type Primitive string

const (
	PrimitiveRename      Primitive = "rename"
	PrimitiveWrap        Primitive = "wrap"
	PrimitiveUnwrap      Primitive = "unwrap"
	PrimitiveSplit       Primitive = "split"
	PrimitiveJoin        Primitive = "join"
	PrimitiveEnum        Primitive = "enum"
	PrimitiveConditional Primitive = "conditional"
	PrimitivePresence    Primitive = "presence"
)

type UIKind string

const (
	UIGroup   UIKind = "group"
	UIText    UIKind = "text"
	UINumber  UIKind = "number"
	UIBoolean UIKind = "boolean"
	UISelect  UIKind = "select"
	UIJSON    UIKind = "json"
)

// Reference pins a manifest to an immutable repository commit and digest.
type Reference struct {
	repository string
	commit     string
	digest     coreartifact.SHA256
}

func NewReference(repository, commit string, digest coreartifact.SHA256) (Reference, error) {
	reference := Reference{repository: repository, commit: commit, digest: digest}
	if err := reference.Validate(); err != nil {
		return Reference{}, err
	}
	return reference, nil
}

func (reference Reference) Repository() string { return reference.repository }

func (reference Reference) Commit() string { return reference.commit }

func (reference Reference) Digest() coreartifact.SHA256 { return reference.digest }

func (reference Reference) Validate() error {
	if !validRepository(reference.repository) {
		return fmt.Errorf("%w: repository must be an owner/name pair", ErrInvalidReference)
	}
	if !validCommit(reference.commit) {
		return fmt.Errorf("%w: commit must be a non-zero 40 or 64 character hexadecimal object ID", ErrInvalidReference)
	}
	if reference.digest.IsZero() {
		return fmt.Errorf("%w: manifest digest must not be all zeroes", ErrInvalidReference)
	}
	return nil
}

type referenceWire struct {
	Repository string              `json:"repository"`
	Commit     string              `json:"commit"`
	Digest     coreartifact.SHA256 `json:"manifest_sha256"`
}

func (reference Reference) MarshalJSON() ([]byte, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(referenceWire{
		Repository: reference.repository,
		Commit:     reference.commit,
		Digest:     reference.digest,
	})
}

func (reference *Reference) UnmarshalJSON(data []byte) error {
	parsed, err := DecodeReference(data)
	if err != nil {
		return err
	}
	*reference = parsed
	return nil
}

// SemanticFact declares ownership and exact-version coverage for one stable
// canonical concept. CanonicalPath and OwnedPaths are canonical JSON Pointers.
type SemanticFact struct {
	ID             string                 `json:"id"`
	CanonicalPath  string                 `json:"canonical_path"`
	Classification CoverageClassification `json:"classification"`
	OwnedPaths     []string               `json:"owned_paths,omitempty"`
}

// Condition contains the corresponding predicate path for each side of a
// projection. Equals is deliberately restricted by validation to a JSON
// scalar, keeping evaluation deterministic and side-effect free.
type Condition struct {
	CanonicalPath string `json:"canonical_path"`
	VersionPath   string `json:"version_path"`
	Equals        any    `json:"equals"`
}

// Transform is one operation in the ordered projector pipeline. Each
// primitive has a fixed shape validated by Manifest.Validate.
type Transform struct {
	ID        string            `json:"id"`
	FactID    string            `json:"fact_id"`
	Primitive Primitive         `json:"primitive"`
	From      []string          `json:"from"`
	To        []string          `json:"to"`
	Separator string            `json:"separator,omitempty"`
	Key       string            `json:"key,omitempty"`
	Enum      map[string]string `json:"enum,omitempty"`
	When      *Condition        `json:"when,omitempty"`
}

type UIOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type VisibilityCondition struct {
	CanonicalPath string `json:"canonical_path"`
	Equals        any    `json:"equals"`
}

// UIDescriptor is intentionally limited to data needed to render a built-in
// control. It cannot name scripts, components, templates, or remote resources.
type UIDescriptor struct {
	ID          string               `json:"id"`
	FactID      string               `json:"fact_id"`
	Kind        UIKind               `json:"kind"`
	Label       string               `json:"label"`
	Help        string               `json:"help,omitempty"`
	Order       int                  `json:"order,omitempty"`
	Options     []UIOption           `json:"options,omitempty"`
	VisibleWhen *VisibilityCondition `json:"visible_when,omitempty"`
}

// ManifestSpec is the construction and JSON wire shape. NewManifest takes a
// defensive copy; Manifest.Spec returns a defensive copy, so a validated
// Manifest remains stable when callers mutate their input slices or maps.
type ManifestSpec struct {
	SchemaVersion uint32                    `json:"schema_version"`
	CoreVersion   coreartifact.ExactVersion `json:"core_version"`
	SupportLevel  SupportLevel              `json:"support_level"`
	SemanticFacts []SemanticFact            `json:"semantic_facts"`
	Transforms    []Transform               `json:"transforms,omitempty"`
	UI            []UIDescriptor            `json:"ui,omitempty"`
}

type Manifest struct {
	spec ManifestSpec
}

func NewManifest(spec ManifestSpec) (*Manifest, error) {
	stable := cloneManifestSpec(spec)
	if err := validateManifestSpec(stable); err != nil {
		return nil, err
	}
	return &Manifest{spec: stable}, nil
}

func (manifest *Manifest) Validate() error {
	if manifest == nil {
		return fmt.Errorf("%w: manifest is nil", ErrInvalidManifest)
	}
	return validateManifestSpec(manifest.spec)
}

func (manifest *Manifest) Spec() ManifestSpec {
	if manifest == nil {
		return ManifestSpec{}
	}
	return cloneManifestSpec(manifest.spec)
}

func (manifest *Manifest) CoreVersion() coreartifact.ExactVersion {
	if manifest == nil {
		return coreartifact.ExactVersion{}
	}
	return manifest.spec.CoreVersion
}

func (manifest *Manifest) SupportLevel() SupportLevel {
	if manifest == nil {
		return ""
	}
	return manifest.spec.SupportLevel
}

func (manifest *Manifest) SemanticFacts() []SemanticFact {
	return manifest.Spec().SemanticFacts
}

func (manifest *Manifest) Transforms() []Transform {
	return manifest.Spec().Transforms
}

func (manifest *Manifest) UIDescriptors() []UIDescriptor {
	return manifest.Spec().UI
}

func (manifest Manifest) MarshalJSON() ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(manifest.spec)
}

func (manifest *Manifest) UnmarshalJSON(data []byte) error {
	parsed, err := DecodeManifest(data)
	if err != nil {
		return err
	}
	*manifest = *parsed
	return nil
}

func cloneManifestSpec(source ManifestSpec) ManifestSpec {
	clone := source
	clone.SemanticFacts = make([]SemanticFact, len(source.SemanticFacts))
	for index, fact := range source.SemanticFacts {
		clone.SemanticFacts[index] = fact
		clone.SemanticFacts[index].OwnedPaths = append([]string(nil), fact.OwnedPaths...)
	}
	clone.Transforms = make([]Transform, len(source.Transforms))
	for index, transform := range source.Transforms {
		clone.Transforms[index] = transform
		clone.Transforms[index].From = append([]string(nil), transform.From...)
		clone.Transforms[index].To = append([]string(nil), transform.To...)
		if transform.Enum != nil {
			clone.Transforms[index].Enum = make(map[string]string, len(transform.Enum))
			for key, value := range transform.Enum {
				clone.Transforms[index].Enum[key] = value
			}
		}
		if transform.When != nil {
			condition := *transform.When
			clone.Transforms[index].When = &condition
		}
	}
	clone.UI = make([]UIDescriptor, len(source.UI))
	for index, descriptor := range source.UI {
		clone.UI[index] = descriptor
		clone.UI[index].Options = append([]UIOption(nil), descriptor.Options...)
		if descriptor.VisibleWhen != nil {
			condition := *descriptor.VisibleWhen
			clone.UI[index].VisibleWhen = &condition
		}
	}
	return clone
}
