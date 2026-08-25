// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"sort"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// ManifestFile is one complete standalone manifest supplied by an offline
// authoring boundary. Name must be the canonical <major>.<minor>.<patch>.json
// base name; Data is decoded using the same strict contract as runtime input.
type ManifestFile struct {
	Name string
	Data []byte
}

// BuildGeneration validates, canonicalizes, hashes, and deterministically
// orders a complete set of standalone exact-version manifests. It performs no
// filesystem or network access.
func BuildGeneration(commit string, files []ManifestFile) (*Generation, error) {
	if !validCommit(commit) {
		return nil, generationError("commit must be a non-zero lowercase 40 or 64 character hexadecimal object ID")
	}
	if len(files) == 0 {
		return nil, generationError("manifest files must not be empty")
	}
	if len(files) > MaximumGenerationEntries {
		return nil, generationError("manifest files exceeds %d entries", MaximumGenerationEntries)
	}

	entries := make([]GenerationManifest, 0, len(files))
	versions := make(map[string]struct{}, len(files))
	for index, file := range files {
		name := file.Name
		versionName, hasJSONSuffix := strings.CutSuffix(name, ".json")
		fileVersion, versionErr := coreartifact.ParseExactVersion(versionName)
		if name == "" || strings.ContainsAny(name, `/\`) || !hasJSONSuffix ||
			versionErr != nil || fileVersion.String() != versionName {
			return nil, generationError(
				"manifest files[%d].name %q must be <major>.<minor>.<patch>.json",
				index,
				name,
			)
		}

		manifest, err := DecodeManifest(file.Data)
		if err != nil {
			return nil, generationError("manifest file %q: %v", name, err)
		}
		version := manifest.CoreVersion().String()
		if manifest.CoreVersion().Compare(fileVersion) != 0 {
			return nil, generationError(
				"manifest file %q declares core_version %q and must be named %q",
				name,
				version,
				version+".json",
			)
		}
		if _, duplicate := versions[version]; duplicate {
			return nil, generationError("duplicate exact core version %q", version)
		}
		versions[version] = struct{}{}

		digest, err := manifest.Digest()
		if err != nil {
			return nil, generationError("digest manifest file %q: %v", name, err)
		}
		entries = append(entries, GenerationManifest{
			path:     "capabilities/" + name,
			digest:   digest,
			manifest: manifest,
		})
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].manifest.CoreVersion().Compare(entries[right].manifest.CoreVersion()) < 0
	})
	generation := &Generation{
		repository: ManifestRepository,
		commit:     commit,
		manifests:  entries,
	}
	canonical, err := generation.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	if len(canonical) > MaximumGenerationBytes {
		return nil, generationError("canonical generation exceeds %d bytes", MaximumGenerationBytes)
	}
	return generation, nil
}
