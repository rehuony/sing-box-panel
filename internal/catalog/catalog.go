// SPDX-License-Identifier: GPL-3.0-or-later

// Package catalog discovers exact stable sing-box release artifacts from the
// fixed SagerNet/sing-box GitHub repository.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrCatalog          = errors.New("catalog operation failed")
	ErrDigestMissing    = errors.New("trusted artifact digest is missing")
	ErrDigestMismatch   = errors.New("trusted artifact digests do not match")
	ErrInvalidCandidate = errors.New("invalid catalog artifact candidate")
)

type Step string

const (
	StepReleases   Step = "releases"
	StepRepository Step = "repository"
	StepDigest     Step = "digest"
	StepFilter     Step = "filter"
)

// Failure has a log-safe Error string. Cause remains available to trusted
// callers through errors.Is/As without being interpolated into logs by default.
type Failure struct {
	Step  Step
	Code  string
	Cause error
}

func (failure *Failure) Error() string {
	return fmt.Sprintf("catalog %s failed (%s)", failure.Step, failure.Code)
}

func (failure *Failure) Unwrap() error { return failure.Cause }

func fail(step Step, code string, cause error) error {
	return &Failure{Step: step, Code: code, Cause: errors.Join(ErrCatalog, cause)}
}

type DiagnosticSeverity string

const (
	DiagnosticInfo    DiagnosticSeverity = "info"
	DiagnosticWarning DiagnosticSeverity = "warning"
)

type Diagnostic struct {
	Step     Step               `json:"step"`
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
}

type DigestKey struct {
	RepositoryID int64                     `json:"repository_id"`
	ReleaseID    int64                     `json:"release_id"`
	AssetID      int64                     `json:"asset_id"`
	Version      coreartifact.ExactVersion `json:"version"`
	AssetName    string                    `json:"asset_name"`
}

// DigestLookup represents the separately maintained, trusted catalog digest
// source. A nil lookup means no project catalog digest is available.
type DigestLookup interface {
	Lookup(ctx context.Context, key DigestKey) (coreartifact.SHA256, bool, error)
}

type Asset struct {
	RepositoryID     int64                        `json:"repository_id"`
	ReleaseID        int64                        `json:"release_id"`
	AssetID          int64                        `json:"asset_id"`
	Name             string                       `json:"name"`
	DownloadURL      string                       `json:"download_url"`
	Size             int64                        `json:"size"`
	Version          coreartifact.ExactVersion    `json:"version"`
	OperatingSystem  coreartifact.OperatingSystem `json:"os"`
	Architecture     coreartifact.Architecture    `json:"arch"`
	Variant          coreartifact.Variant         `json:"variant"`
	APIDigest        coreartifact.SHA256          `json:"api_digest,omitempty"`
	HasAPIDigest     bool                         `json:"has_api_digest"`
	CatalogDigest    coreartifact.SHA256          `json:"catalog_digest,omitempty"`
	HasCatalogDigest bool                         `json:"has_catalog_digest"`
}

// TrustedDigest returns the one SHA-256 value an official installation may
// trust. Missing evidence and disagreement are both hard failures.
func (asset Asset) TrustedDigest() (coreartifact.SHA256, error) {
	if asset.HasAPIDigest && asset.APIDigest.IsZero() {
		return coreartifact.SHA256{}, fmt.Errorf("%w: API digest is zero", ErrInvalidCandidate)
	}
	if asset.HasCatalogDigest && asset.CatalogDigest.IsZero() {
		return coreartifact.SHA256{}, fmt.Errorf("%w: catalog digest is zero", ErrInvalidCandidate)
	}
	if asset.HasAPIDigest && asset.HasCatalogDigest && asset.APIDigest != asset.CatalogDigest {
		return coreartifact.SHA256{}, ErrDigestMismatch
	}
	if asset.HasCatalogDigest {
		return asset.CatalogDigest, nil
	}
	if asset.HasAPIDigest {
		return asset.APIDigest, nil
	}
	return coreartifact.SHA256{}, ErrDigestMissing
}

func (asset Asset) Validate() error {
	if asset.RepositoryID != OfficialRepositoryID || asset.ReleaseID <= 0 || asset.AssetID <= 0 {
		return fmt.Errorf("%w: pinned repository and positive release/asset IDs are required", ErrInvalidCandidate)
	}
	if asset.Name == "" || len(asset.Name) > 512 || strings.TrimSpace(asset.Name) != asset.Name {
		return fmt.Errorf("%w: asset name is invalid", ErrInvalidCandidate)
	}
	if !validOfficialDownloadURL(asset.DownloadURL, asset.Version, asset.Name) || asset.Size <= 0 || asset.Version.IsZero() {
		return fmt.Errorf("%w: URL, positive size, and exact version are required", ErrInvalidCandidate)
	}
	architecture, variant, valid := classifyAsset(asset.Version, asset.Name)
	if !valid || architecture != asset.Architecture || variant != asset.Variant || asset.OperatingSystem != coreartifact.OperatingSystemLinux {
		return fmt.Errorf("%w: asset name and platform dimensions disagree", ErrInvalidCandidate)
	}
	if (!asset.HasAPIDigest && !asset.APIDigest.IsZero()) || (!asset.HasCatalogDigest && !asset.CatalogDigest.IsZero()) {
		return fmt.Errorf("%w: digest value and presence flag disagree", ErrInvalidCandidate)
	}
	source, err := coreartifact.NewOfficialSource(asset.RepositoryID, asset.ReleaseID, asset.AssetID)
	if err != nil {
		return fmt.Errorf("%w: source identity", ErrInvalidCandidate)
	}
	digest := asset.APIDigest
	if digest.IsZero() {
		digest = asset.CatalogDigest
	}
	if digest.IsZero() {
		// A candidate may be displayed without a digest, but validate its other
		// identity dimensions with a non-zero placeholder.
		parsed, parseErr := coreartifact.ParseSHA256(strings.Repeat("01", 32))
		if parseErr != nil {
			return fmt.Errorf("%w: internal digest placeholder", ErrInvalidCandidate)
		}
		digest = parsed
	}
	if _, err := coreartifact.NewIdentity(
		source,
		digest,
		asset.OperatingSystem,
		asset.Architecture,
		asset.Variant,
		asset.Version,
	); err != nil {
		return fmt.Errorf("%w: artifact dimensions", ErrInvalidCandidate)
	}
	return nil
}

type Release struct {
	ID      int64                     `json:"id"`
	Tag     string                    `json:"tag"`
	Version coreartifact.ExactVersion `json:"version"`
	Assets  []Asset                   `json:"assets"`
}

type Catalog struct {
	RepositoryID int64     `json:"repository_id"`
	Releases     []Release `json:"releases"`
}

func (catalog Catalog) Assets() []Asset {
	count := 0
	for _, release := range catalog.Releases {
		count += len(release.Assets)
	}
	assets := make([]Asset, 0, count)
	for _, release := range catalog.Releases {
		assets = append(assets, release.Assets...)
	}
	return assets
}

type RefreshResult struct {
	Catalog Catalog `json:"catalog"`
	// ETag is an opaque validator for the complete paginated snapshot. Persist
	// and pass it back unchanged; it is not a single GitHub page's HTTP ETag.
	ETag        string       `json:"validator"`
	NotModified bool         `json:"not_modified"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func sortCatalog(result *Catalog) {
	sort.Slice(result.Releases, func(left, right int) bool {
		return result.Releases[left].Version.Compare(result.Releases[right].Version) > 0
	})
	for releaseIndex := range result.Releases {
		assets := result.Releases[releaseIndex].Assets
		sort.Slice(assets, func(left, right int) bool {
			if assets[left].Architecture != assets[right].Architecture {
				return assets[left].Architecture < assets[right].Architecture
			}
			if assets[left].Variant != assets[right].Variant {
				return assets[left].Variant < assets[right].Variant
			}
			if assets[left].Name != assets[right].Name {
				return assets[left].Name < assets[right].Name
			}
			return assets[left].AssetID < assets[right].AssetID
		})
		result.Releases[releaseIndex].Assets = assets
	}
}
