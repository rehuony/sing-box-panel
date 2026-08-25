// SPDX-License-Identifier: GPL-3.0-or-later

// Package coreartifact defines immutable identities for installable sing-box
// core bytes and their exact reported versions.
package coreartifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maximumIdentityJSONBytes = 16 << 10

var ErrInvalidIdentity = errors.New("invalid core artifact identity")

type SourceKind string

const (
	SourceOfficial SourceKind = "official"
	SourceUser     SourceKind = "user_verified"
)

type OperatingSystem string

const OperatingSystemLinux OperatingSystem = "linux"

type Architecture string

const (
	ArchitectureAMD64 Architecture = "amd64"
	ArchitectureARM64 Architecture = "arm64"
)

type Variant string

const (
	VariantPlain Variant = "plain"
	VariantGlibc Variant = "glibc"
	VariantMusl  Variant = "musl"
)

// Source identifies either an immutable GitHub release asset or a
// user-supplied import. A user source is descriptive metadata and is never a
// filesystem authority.
type Source struct {
	kind         SourceKind
	repositoryID int64
	releaseID    int64
	assetID      int64
	userSource   string
}

func NewOfficialSource(repositoryID, releaseID, assetID int64) (Source, error) {
	source := Source{
		kind:         SourceOfficial,
		repositoryID: repositoryID,
		releaseID:    releaseID,
		assetID:      assetID,
	}
	if err := source.validate(); err != nil {
		return Source{}, err
	}
	return source, nil
}

func NewUserSource(description string) (Source, error) {
	source := Source{kind: SourceUser, userSource: description}
	if err := source.validate(); err != nil {
		return Source{}, err
	}
	return source, nil
}

func (source Source) Kind() SourceKind { return source.kind }

func (source Source) RepositoryID() int64 { return source.repositoryID }

func (source Source) ReleaseID() int64 { return source.releaseID }

func (source Source) AssetID() int64 { return source.assetID }

func (source Source) UserSource() string { return source.userSource }

func (source Source) validate() error {
	switch source.kind {
	case SourceOfficial:
		if source.repositoryID <= 0 || source.releaseID <= 0 || source.assetID <= 0 {
			return fmt.Errorf("%w: official source IDs must all be positive", ErrInvalidIdentity)
		}
		if source.userSource != "" {
			return fmt.Errorf("%w: official source cannot contain user source metadata", ErrInvalidIdentity)
		}
	case SourceUser:
		if source.repositoryID != 0 || source.releaseID != 0 || source.assetID != 0 {
			return fmt.Errorf("%w: user source cannot contain official IDs", ErrInvalidIdentity)
		}
		if err := validateDescription(source.userSource); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unsupported source kind %q", ErrInvalidIdentity, source.kind)
	}
	return nil
}

// Identity is the immutable identity of bytes eligible for installation and
// execution. SHA-256, platform, variant, and reported version are all part of
// the identity; semantic version alone is deliberately insufficient.
type Identity struct {
	source          Source
	digest          SHA256
	operatingSystem OperatingSystem
	architecture    Architecture
	variant         Variant
	reportedVersion ExactVersion
}

func NewIdentity(
	source Source,
	digest SHA256,
	operatingSystem OperatingSystem,
	architecture Architecture,
	variant Variant,
	reportedVersion ExactVersion,
) (Identity, error) {
	identity := Identity{
		source:          source,
		digest:          digest,
		operatingSystem: operatingSystem,
		architecture:    architecture,
		variant:         variant,
		reportedVersion: reportedVersion,
	}
	if err := identity.Validate(); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func (identity Identity) Source() Source { return identity.source }

func (identity Identity) Digest() SHA256 { return identity.digest }

func (identity Identity) OperatingSystem() OperatingSystem { return identity.operatingSystem }

func (identity Identity) Architecture() Architecture { return identity.architecture }

func (identity Identity) Variant() Variant { return identity.variant }

func (identity Identity) ReportedVersion() ExactVersion { return identity.reportedVersion }

func (identity Identity) Validate() error {
	if err := identity.source.validate(); err != nil {
		return err
	}
	if identity.digest.IsZero() {
		return fmt.Errorf("%w: SHA-256 must not be all zeroes", ErrInvalidIdentity)
	}
	if identity.operatingSystem != OperatingSystemLinux {
		return fmt.Errorf("%w: unsupported operating system %q", ErrInvalidIdentity, identity.operatingSystem)
	}
	switch identity.architecture {
	case ArchitectureAMD64, ArchitectureARM64:
	default:
		return fmt.Errorf("%w: unsupported architecture %q", ErrInvalidIdentity, identity.architecture)
	}
	if !validVariant(identity.variant) {
		return fmt.Errorf("%w: variant %q is not a safe artifact variant identifier", ErrInvalidIdentity, identity.variant)
	}
	if identity.reportedVersion.IsZero() {
		return fmt.Errorf("%w: reported version must not be 0.0.0", ErrInvalidIdentity)
	}
	return nil
}

func validVariant(variant Variant) bool {
	if len(variant) == 0 || len(variant) > 64 || variant[0] < 'a' || variant[0] > 'z' {
		return false
	}
	for _, character := range variant {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

type identityWire struct {
	SourceKind      SourceKind      `json:"source_kind"`
	RepositoryID    int64           `json:"repository_id,omitempty"`
	ReleaseID       int64           `json:"release_id,omitempty"`
	AssetID         int64           `json:"asset_id,omitempty"`
	UserSource      string          `json:"user_source,omitempty"`
	SHA256          SHA256          `json:"sha256"`
	OperatingSystem OperatingSystem `json:"os"`
	Architecture    Architecture    `json:"arch"`
	Variant         Variant         `json:"variant"`
	ReportedVersion ExactVersion    `json:"reported_version"`
}

func (identity Identity) MarshalJSON() ([]byte, error) {
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(identity.toWire())
}

func (identity *Identity) UnmarshalJSON(data []byte) error {
	parsed, err := DecodeIdentity(data)
	if err != nil {
		return err
	}
	*identity = parsed
	return nil
}

func DecodeIdentity(data []byte) (Identity, error) {
	if err := validateJSONObject(data, maximumIdentityJSONBytes, 8); err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrInvalidIdentity, err)
	}
	var wire identityWire
	if err := strictDecode(data, &wire); err != nil {
		return Identity{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidIdentity, err)
	}

	source := Source{
		kind:         wire.SourceKind,
		repositoryID: wire.RepositoryID,
		releaseID:    wire.ReleaseID,
		assetID:      wire.AssetID,
		userSource:   wire.UserSource,
	}
	return NewIdentity(
		source,
		wire.SHA256,
		wire.OperatingSystem,
		wire.Architecture,
		wire.Variant,
		wire.ReportedVersion,
	)
}

func (identity Identity) toWire() identityWire {
	return identityWire{
		SourceKind:      identity.source.kind,
		RepositoryID:    identity.source.repositoryID,
		ReleaseID:       identity.source.releaseID,
		AssetID:         identity.source.assetID,
		UserSource:      identity.source.userSource,
		SHA256:          identity.digest,
		OperatingSystem: identity.operatingSystem,
		Architecture:    identity.architecture,
		Variant:         identity.variant,
		ReportedVersion: identity.reportedVersion,
	}
}

func validateDescription(value string) error {
	if value == "" {
		return fmt.Errorf("%w: user source description is required", ErrInvalidIdentity)
	}
	if len(value) > 512 {
		return fmt.Errorf("%w: user source description exceeds 512 bytes", ErrInvalidIdentity)
	}
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: user source description must be valid, trimmed UTF-8", ErrInvalidIdentity)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: user source description contains a control character", ErrInvalidIdentity)
		}
	}
	return nil
}
