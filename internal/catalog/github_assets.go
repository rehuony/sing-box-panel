// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"context"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

type githubRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type githubRelease struct {
	ID         int64         `json:"id"`
	TagName    string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func (client *GitHubClient) filter(ctx context.Context, repositoryID int64, releases []githubRelease) ([]Release, []Diagnostic, error) {
	if len(releases) > 5000 {
		return nil, nil, fail(StepFilter, "release_count", nil)
	}
	filtered := make([]Release, 0, len(releases))
	diagnostics := make([]Diagnostic, 0)
	seenVersions := make(map[coreartifact.ExactVersion]struct{})
	seenReleases := make(map[int64]struct{})
	seenAssets := make(map[int64]struct{})
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			diagnostics = append(diagnostics, Diagnostic{Step: StepFilter, Severity: DiagnosticInfo, Code: "unstable_release_skipped", Message: "a draft or prerelease entry was skipped"})
			continue
		}
		version, valid := stableVersion(release.TagName)
		if !valid || release.ID <= 0 {
			diagnostics = append(diagnostics, Diagnostic{Step: StepFilter, Severity: DiagnosticWarning, Code: "invalid_release_skipped", Message: "a release without a strict stable tag or identity was skipped"})
			continue
		}
		if _, duplicate := seenVersions[version]; duplicate {
			return nil, nil, fail(StepFilter, "duplicate_version", nil)
		}
		seenVersions[version] = struct{}{}
		if _, duplicate := seenReleases[release.ID]; duplicate {
			return nil, nil, fail(StepFilter, "duplicate_release_identity", nil)
		}
		seenReleases[release.ID] = struct{}{}
		if len(release.Assets) > 5000 {
			return nil, nil, fail(StepFilter, "asset_count", nil)
		}
		candidateRelease := Release{ID: release.ID, Tag: release.TagName, Version: version, Assets: make([]Asset, 0)}
		for _, rawAsset := range release.Assets {
			if _, duplicate := seenAssets[rawAsset.ID]; duplicate || rawAsset.ID <= 0 {
				return nil, nil, fail(StepFilter, "duplicate_asset_identity", nil)
			}
			seenAssets[rawAsset.ID] = struct{}{}
			architecture, variant, valid := classifyAsset(version, rawAsset.Name)
			if !valid {
				continue
			}
			if rawAsset.Size <= 0 || !validOfficialDownloadURL(rawAsset.BrowserDownloadURL, version, rawAsset.Name) {
				diagnostics = append(diagnostics, Diagnostic{Step: StepFilter, Severity: DiagnosticWarning, Code: "invalid_asset_skipped", Message: "a Linux artifact with invalid size or URL was skipped"})
				continue
			}
			candidate := Asset{
				RepositoryID: repositoryID, ReleaseID: release.ID, AssetID: rawAsset.ID,
				Name: rawAsset.Name, DownloadURL: rawAsset.BrowserDownloadURL, Size: rawAsset.Size,
				Version: version, OperatingSystem: coreartifact.OperatingSystemLinux,
				Architecture: architecture, Variant: variant,
			}
			if rawAsset.Digest != "" {
				apiDigest, err := parseGitHubDigest(rawAsset.Digest)
				if err != nil {
					diagnostics = append(diagnostics, Diagnostic{Step: StepDigest, Severity: DiagnosticWarning, Code: "invalid_api_digest", Message: "an artifact with a malformed API digest was skipped"})
					continue
				}
				candidate.APIDigest, candidate.HasAPIDigest = apiDigest, true
			}
			if client.digestLookup != nil {
				catalogDigest, found, err := client.digestLookup.Lookup(ctx, DigestKey{
					RepositoryID: repositoryID, ReleaseID: release.ID, AssetID: rawAsset.ID,
					Version: version, AssetName: rawAsset.Name,
				})
				if err != nil {
					return nil, nil, fail(StepDigest, "lookup_failed", err)
				}
				if found {
					if catalogDigest.IsZero() {
						return nil, nil, fail(StepDigest, "invalid_catalog_digest", nil)
					}
					candidate.CatalogDigest, candidate.HasCatalogDigest = catalogDigest, true
				}
			}
			if err := candidate.Validate(); err != nil {
				return nil, nil, fail(StepFilter, "invalid_candidate", err)
			}
			if _, err := candidate.TrustedDigest(); err != nil {
				code := "digest_missing"
				if errors.Is(err, ErrDigestMismatch) {
					code = "digest_mismatch"
				}
				diagnostics = append(diagnostics, Diagnostic{Step: StepDigest, Severity: DiagnosticWarning, Code: code, Message: "artifact is visible but official installation is blocked by digest evidence"})
			}
			candidateRelease.Assets = append(candidateRelease.Assets, candidate)
		}
		filtered = append(filtered, candidateRelease)
	}
	return filtered, diagnostics, nil
}
