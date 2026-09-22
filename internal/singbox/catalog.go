// SPDX-License-Identifier: GPL-3.0-or-later

// Package singbox owns the immutable, reviewed sing-box support policy.
// The source catalog is used by repository tooling; production consumes
// generated Go values compiled into the binary.
package singbox

import "slices"

const (
	SchemaSourceNative      = "native"
	SchemaSourceReviewed113 = "reviewed-1.13"
	ArchitectureAMD64       = "amd64"
	ArchitectureARM64       = "arm64"
)

type Upstream struct {
	Tag       string `json:"tag"`
	Commit    string `json:"commit"`
	ModuleSum string `json:"module_sum"`
	GoModSum  string `json:"go_mod_sum"`
}

type Profile struct {
	AssetName string   `json:"asset_name"`
	URL       string   `json:"url"`
	SHA256    string   `json:"sha256"`
	Size      int64    `json:"size"`
	Features  []string `json:"features"`
}

type Version struct {
	ExactVersion string `json:"version"`
	// InboundFamily is present only when this exact version has a compiled
	// subscription inbound converter. Runtime and Schema support do not depend
	// on this optional capability.
	SchemaSource  string             `json:"configuration_schema,omitempty"`
	InboundFamily string             `json:"inbound_family,omitempty"`
	Upstream      Upstream           `json:"upstream"`
	Profiles      map[string]Profile `json:"profiles"`
}

func Versions() []Version {
	result := make([]Version, len(generatedVersions))
	for index, version := range generatedVersions {
		result[index] = cloneVersion(version)
	}
	return result
}

func Lookup(exactVersion string) (Version, bool) {
	for _, version := range generatedVersions {
		if version.ExactVersion == exactVersion {
			return cloneVersion(version), true
		}
	}
	return Version{}, false
}

func cloneVersion(value Version) Version {
	profiles := value.Profiles
	value.Profiles = make(map[string]Profile, len(profiles))
	for architecture, profile := range profiles {
		profile.Features = slices.Clone(profile.Features)
		value.Profiles[architecture] = profile
	}
	return value
}
