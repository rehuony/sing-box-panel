// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

const maximumCatalogBytes = 64 << 20

type catalogRefresher interface {
	Refresh(context.Context, string) (RefreshResult, error)
}

type CatalogSnapshot struct {
	Catalog     Catalog      `json:"catalog"`
	Validator   string       `json:"validator"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	RefreshedAt time.Time    `json:"refreshed_at"`
	NotModified bool         `json:"not_modified"`
}

type CatalogAssetFilter struct {
	ExactVersion string
	Architecture string
	Variant      string
}

type CatalogAssetList struct {
	Validator   string    `json:"validator"`
	RefreshedAt time.Time `json:"refreshed_at"`
	Assets      []Asset   `json:"assets"`
}

type CoreArtifact struct {
	ID                 string                       `json:"id"`
	ExactVersion       string                       `json:"exact_version"`
	OperatingSystem    string                       `json:"os"`
	Architecture       string                       `json:"arch"`
	Variant            string                       `json:"variant"`
	SourceKind         store.CoreArtifactSourceKind `json:"source_kind"`
	UserSource         string                       `json:"user_source,omitempty"`
	RepositoryID       int64                        `json:"repository_id,omitempty"`
	ReleaseID          int64                        `json:"release_id,omitempty"`
	AssetID            int64                        `json:"asset_id,omitempty"`
	ArchiveSHA256      string                       `json:"archive_sha256"`
	BinarySHA256       string                       `json:"binary_sha256"`
	BinaryPath         string                       `json:"binary_path"`
	ReportedVersion    string                       `json:"reported_version"`
	FeatureFingerprint json.RawMessage              `json:"feature_fingerprint"`
	CreatedAt          time.Time                    `json:"created_at"`
}

type CoreArtifactListFilter struct {
	ExactVersion string
	Architecture string
	Variant      string
	SourceKind   store.CoreArtifactSourceKind
	Cursor       *CoreArtifactCursor
	Limit        int
}

type CoreArtifactCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type CoreArtifactPage struct {
	Items []CoreArtifact      `json:"items"`
	Next  *CoreArtifactCursor `json:"next,omitempty"`
}

type CatalogRefreshOptions struct {
	Force bool `json:"force"`
}

func (application *Service) RefreshCatalog(ctx context.Context, options CatalogRefreshOptions) (result CatalogSnapshot, operationErr error) {
	defer func() {
		application.telemetry.RecordOperation(ctx, "catalog.refresh", "Catalog refresh", operationErr, telemetry.OperationLogContext{})
	}()
	application.catalogMu.Lock()
	defer application.catalogMu.Unlock()
	currentSettings, err := application.catalogSettings(ctx)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	if !options.Force && currentSettings.CatalogRefreshIntervalHours > 0 {
		state, stateErr := application.database.CatalogState(ctx)
		if stateErr == nil && application.now().UTC().Before(
			state.RefreshedAt.Add(time.Duration(currentSettings.CatalogRefreshIntervalHours)*time.Hour),
		) {
			snapshot, catalogErr := application.Catalog(ctx)
			if catalogErr != nil {
				return CatalogSnapshot{}, catalogErr
			}
			snapshot.NotModified = true
			snapshot.Diagnostics = append(snapshot.Diagnostics, Diagnostic{
				Step: StepReleases, Severity: DiagnosticInfo, Code: "refresh_not_due",
				Message: "the next scheduled official catalog refresh is not due yet",
			})
			return snapshot, nil
		}
		if stateErr != nil && !errors.Is(stateErr, store.ErrCatalogStateNotFound) {
			return CatalogSnapshot{}, stateErr
		}
	}
	client, err := NewGitHubClient(ClientOptions{Token: currentSettings.Token})
	if err != nil {
		return CatalogSnapshot{}, err
	}
	return application.refreshCatalogWith(ctx, client)
}

func (application *Service) refreshCatalogWith(ctx context.Context, client catalogRefresher) (CatalogSnapshot, error) {
	if client == nil {
		return CatalogSnapshot{}, errors.New("catalog refresher is unavailable")
	}
	var previous *store.CatalogState
	stored, err := application.database.CatalogState(ctx)
	if err == nil {
		previous = &stored
	} else if !errors.Is(err, store.ErrCatalogStateNotFound) {
		return CatalogSnapshot{}, err
	}
	validator := ""
	if previous != nil {
		validator = previous.Validator
	}
	result, err := client.Refresh(ctx, validator)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	currentCatalog := result.Catalog
	if result.NotModified {
		if previous == nil {
			return CatalogSnapshot{}, errors.New("upstream returned not-modified without a local catalog")
		}
		currentCatalog, err = decodeCatalog(previous.Catalog)
		if err != nil {
			return CatalogSnapshot{}, err
		}
		result.ETag = previous.Validator
	}
	if err := validateCatalog(currentCatalog); err != nil {
		return CatalogSnapshot{}, err
	}
	catalogJSON, err := json.Marshal(currentCatalog)
	if err != nil {
		return CatalogSnapshot{}, fmt.Errorf("encode catalog snapshot: %w", err)
	}
	diagnosticsJSON, err := json.Marshal(result.Diagnostics)
	if err != nil {
		return CatalogSnapshot{}, fmt.Errorf("encode catalog diagnostics: %w", err)
	}
	refreshedAt := application.now().UTC()
	if _, err := application.database.SaveCatalogState(ctx, store.CatalogState{
		Validator: result.ETag, Catalog: catalogJSON, Diagnostics: diagnosticsJSON, RefreshedAt: refreshedAt,
	}); err != nil {
		return CatalogSnapshot{}, err
	}
	return CatalogSnapshot{
		Catalog: currentCatalog, Validator: result.ETag, Diagnostics: result.Diagnostics,
		RefreshedAt: refreshedAt, NotModified: result.NotModified,
	}, nil
}

func (application *Service) Catalog(ctx context.Context) (CatalogSnapshot, error) {
	state, err := application.database.CatalogState(ctx)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	decoded, err := decodeCatalog(state.Catalog)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	var diagnostics []Diagnostic
	if err := jsonstrict.Decode(state.Diagnostics, maximumCatalogBytes, &diagnostics); err != nil {
		return CatalogSnapshot{}, fmt.Errorf("decode catalog diagnostics: %w", err)
	}
	return CatalogSnapshot{
		Catalog: decoded, Validator: state.Validator, Diagnostics: diagnostics, RefreshedAt: state.RefreshedAt,
	}, nil
}

func (application *Service) ListCatalogAssets(
	ctx context.Context,
	filter CatalogAssetFilter,
) (CatalogAssetList, error) {
	snapshot, err := application.Catalog(ctx)
	if err != nil {
		return CatalogAssetList{}, err
	}
	if filter.ExactVersion != "" {
		if _, err := coreartifact.ParseExactVersion(filter.ExactVersion); err != nil {
			return CatalogAssetList{}, err
		}
	}
	assets := make([]Asset, 0)
	for _, asset := range snapshot.Catalog.Assets() {
		if filter.ExactVersion != "" && asset.Version.String() != filter.ExactVersion {
			continue
		}
		if filter.Architecture != "" && string(asset.Architecture) != filter.Architecture {
			continue
		}
		if filter.Variant != "" && string(asset.Variant) != filter.Variant {
			continue
		}
		assets = append(assets, asset)
	}
	sort.SliceStable(assets, func(left, right int) bool {
		comparison := assets[left].Version.Compare(assets[right].Version)
		if comparison != 0 {
			return comparison > 0
		}
		return assets[left].AssetID < assets[right].AssetID
	})
	return CatalogAssetList{Validator: snapshot.Validator, RefreshedAt: snapshot.RefreshedAt, Assets: assets}, nil
}

func decodeCatalog(raw json.RawMessage) (Catalog, error) {
	var decoded Catalog
	if err := jsonstrict.Decode(raw, maximumCatalogBytes, &decoded); err != nil {
		return Catalog{}, fmt.Errorf("decode stored catalog: %w", err)
	}
	if err := validateCatalog(decoded); err != nil {
		return Catalog{}, err
	}
	return decoded, nil
}

func validateCatalog(value Catalog) error {
	if value.RepositoryID != OfficialRepositoryID {
		return errors.New("catalog repository identity is invalid")
	}
	seenReleases := make(map[int64]struct{})
	seenAssets := make(map[int64]struct{})
	for _, release := range value.Releases {
		if release.ID <= 0 || release.Tag != "v"+release.Version.String() {
			return errors.New("catalog release identity is invalid")
		}
		if _, duplicate := seenReleases[release.ID]; duplicate {
			return errors.New("catalog contains a duplicate release")
		}
		seenReleases[release.ID] = struct{}{}
		for _, asset := range release.Assets {
			if asset.ReleaseID != release.ID || asset.RepositoryID != value.RepositoryID || asset.Version != release.Version {
				return errors.New("catalog asset does not match its release")
			}
			if _, duplicate := seenAssets[asset.AssetID]; duplicate {
				return errors.New("catalog contains a duplicate asset")
			}
			seenAssets[asset.AssetID] = struct{}{}
			if err := asset.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// CoreArtifactFromStore converts a recorded core artifact to its API view.
func CoreArtifactFromStore(value store.CoreArtifact) CoreArtifact {
	return CoreArtifact{
		ID: value.ID, ExactVersion: value.ExactVersion, OperatingSystem: value.OperatingSystem,
		Architecture: value.Architecture, Variant: value.Variant, SourceKind: value.SourceKind,
		UserSource: value.UserSource, RepositoryID: value.RepositoryID, ReleaseID: value.ReleaseID,
		AssetID: value.AssetID, ArchiveSHA256: value.ArchiveSHA256, BinarySHA256: value.BinarySHA256,
		BinaryPath:      value.BinaryPath,
		ReportedVersion: value.ReportedVersion, FeatureFingerprint: append(json.RawMessage(nil), value.FeatureFingerprint...),
		CreatedAt: value.CreatedAt,
	}
}

func IsCatalogNotInitialized(err error) bool {
	return errors.Is(err, store.ErrCatalogStateNotFound)
}

func IsCoreArtifactNotFound(err error) bool {
	return errors.Is(err, store.ErrCoreArtifactNotFound)
}

func IsCoreArtifactInUse(err error) bool {
	return errors.Is(err, store.ErrCoreArtifactInUse)
}
