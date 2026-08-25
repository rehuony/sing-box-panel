// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

const (
	// ManifestRepository is the only repository from which executable panel
	// releases accept capability data. Keeping manifests beside the code makes
	// review and rollback use the same immutable Git object.
	ManifestRepository = "rehuony/sing-box-panel"

	GenerationSchemaVersion  = 1
	MaximumGenerationBytes   = 64 << 20
	MaximumGenerationEntries = 4096
)

var ErrInvalidGeneration = errors.New("invalid capability generation")

// GenerationManifest is one immutable logical file under capabilities/.
// Manifest itself is immutable after construction.
type GenerationManifest struct {
	path     string
	digest   coreartifact.SHA256
	manifest *Manifest
}

func (entry GenerationManifest) Path() string { return entry.path }

func (entry GenerationManifest) Digest() coreartifact.SHA256 { return entry.digest }

func (entry GenerationManifest) Manifest() *Manifest {
	if entry.manifest == nil {
		return nil
	}
	clone, err := NewManifest(entry.manifest.Spec())
	if err != nil {
		// The private source was validated when the generation was built. This
		// branch can only be reached after memory corruption.
		return nil
	}
	return clone
}

// Generation is a complete, commit-bound set of exact-version manifests. It
// contains data only; no entry can name executable code or a remote reference.
type Generation struct {
	repository string
	commit     string
	manifests  []GenerationManifest
}

func (generation *Generation) Repository() string {
	if generation == nil {
		return ""
	}
	return generation.repository
}

func (generation *Generation) Commit() string {
	if generation == nil {
		return ""
	}
	return generation.commit
}

func (generation *Generation) Manifests() []GenerationManifest {
	if generation == nil {
		return nil
	}
	return append([]GenerationManifest(nil), generation.manifests...)
}

type generationWire struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Repository    string                   `json:"repository"`
	CommitSHA     string                   `json:"commit_sha"`
	ManifestCount int                      `json:"manifest_count"`
	Manifests     []generationManifestWire `json:"manifests"`
}

type generationManifestWire struct {
	Path           string          `json:"path"`
	ManifestSHA256 string          `json:"manifest_sha256"`
	Manifest       json.RawMessage `json:"manifest"`
}

// DecodeGeneration validates an entire local generation before callers make
// it visible. The supplied per-file digest covers the canonical standalone
// manifest bytes, independent of formatting in the transport envelope.
func DecodeGeneration(data []byte) (*Generation, error) {
	if err := inspectJSON(data, MaximumGenerationBytes, maximumJSONDepth+4, forbiddenManifestKeys); err != nil {
		return nil, generationError("%v", err)
	}
	var wire generationWire
	if err := strictDecode(data, &wire); err != nil {
		return nil, generationError("decode JSON: %v", err)
	}
	if wire.SchemaVersion != GenerationSchemaVersion {
		return nil, generationError("schema_version must be %d", GenerationSchemaVersion)
	}
	if wire.Repository != ManifestRepository {
		return nil, generationError("repository must be %q", ManifestRepository)
	}
	if !validCommit(wire.CommitSHA) {
		return nil, generationError("commit_sha must be a non-zero lowercase 40 or 64 character hexadecimal object ID")
	}
	if len(wire.Manifests) == 0 {
		return nil, generationError("manifests must not be empty")
	}
	if wire.ManifestCount != len(wire.Manifests) {
		return nil, generationError(
			"manifest_count is %d, but generation contains %d manifests",
			wire.ManifestCount,
			len(wire.Manifests),
		)
	}
	if len(wire.Manifests) > MaximumGenerationEntries {
		return nil, generationError("manifests exceeds %d entries", MaximumGenerationEntries)
	}

	entries := make([]GenerationManifest, 0, len(wire.Manifests))
	versions := make(map[string]struct{}, len(wire.Manifests))
	paths := make(map[string]struct{}, len(wire.Manifests))
	for index, candidate := range wire.Manifests {
		manifest, err := DecodeManifest(candidate.Manifest)
		if err != nil {
			return nil, generationError("manifests[%d]: %v", index, err)
		}
		version := manifest.CoreVersion().String()
		if _, duplicate := versions[version]; duplicate {
			return nil, generationError("duplicate exact core version %q", version)
		}
		versions[version] = struct{}{}

		expectedPath := "capabilities/" + version + ".json"
		if candidate.Path != expectedPath || path.Clean(candidate.Path) != candidate.Path {
			return nil, generationError("manifests[%d].path must be %q", index, expectedPath)
		}
		if _, duplicate := paths[candidate.Path]; duplicate {
			return nil, generationError("duplicate manifest path %q", candidate.Path)
		}
		paths[candidate.Path] = struct{}{}

		declared, err := coreartifact.ParseSHA256(candidate.ManifestSHA256)
		if err != nil || declared.IsZero() {
			return nil, generationError("manifests[%d].manifest_sha256 is invalid or zero", index)
		}
		actual, err := manifest.Digest()
		if err != nil {
			return nil, generationError("manifests[%d] digest: %v", index, err)
		}
		if declared != actual {
			return nil, generationError(
				"manifests[%d] digest mismatch: declared %s, canonical file is %s",
				index,
				declared,
				actual,
			)
		}
		entries = append(entries, GenerationManifest{
			path: candidate.Path, digest: actual, manifest: manifest,
		})
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].manifest.CoreVersion().Compare(entries[right].manifest.CoreVersion()) < 0
	})
	return &Generation{
		repository: wire.Repository,
		commit:     wire.CommitSHA,
		manifests:  entries,
	}, nil
}

func (generation *Generation) CanonicalJSON() ([]byte, error) {
	if generation == nil || generation.repository != ManifestRepository ||
		!validCommit(generation.commit) || len(generation.manifests) == 0 {
		return nil, generationError("generation is incomplete")
	}
	wire := generationWire{
		SchemaVersion: GenerationSchemaVersion,
		Repository:    generation.repository,
		CommitSHA:     generation.commit,
		ManifestCount: len(generation.manifests),
		Manifests:     make([]generationManifestWire, len(generation.manifests)),
	}
	for index, entry := range generation.manifests {
		canonical, err := entry.manifest.CanonicalJSON()
		if err != nil {
			return nil, generationError("canonicalize %q: %v", entry.path, err)
		}
		wire.Manifests[index] = generationManifestWire{
			Path: entry.path, ManifestSHA256: entry.digest.String(), Manifest: canonical,
		}
	}
	return json.Marshal(wire)
}

func (generation *Generation) Digest() (coreartifact.SHA256, error) {
	canonical, err := generation.CanonicalJSON()
	if err != nil {
		return coreartifact.SHA256{}, err
	}
	return coreartifact.NewSHA256(sha256.Sum256(canonical)), nil
}

func generationError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidGeneration, fmt.Sprintf(format, arguments...))
}
