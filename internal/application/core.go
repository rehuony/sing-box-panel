// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const maximumCatalogBytes = 64 << 20

var ErrCoreArtifactVerificationBlocked = errors.New("core artifact verification state blocks installation")

type catalogRefresher interface {
	Refresh(context.Context, string) (catalog.RefreshResult, error)
}

type ArtifactInstaller interface {
	InstallOfficial(context.Context, catalog.Asset) (artifactstore.Result, error)
	ImportLocal(context.Context, artifactstore.ImportRequest) (artifactstore.Result, error)
}

type CatalogSnapshot struct {
	Catalog     catalog.Catalog      `json:"catalog"`
	Validator   string               `json:"validator"`
	Diagnostics []catalog.Diagnostic `json:"diagnostics"`
	RefreshedAt time.Time            `json:"refreshed_at"`
	NotModified bool                 `json:"not_modified"`
}

type CatalogAssetFilter struct {
	ExactVersion string
	Architecture string
	Variant      string
	Installable  bool
}

type CatalogAssetList struct {
	Validator   string          `json:"validator"`
	RefreshedAt time.Time       `json:"refreshed_at"`
	Assets      []catalog.Asset `json:"assets"`
}

type CoreArtifact struct {
	ID                 string                              `json:"id"`
	ExactVersion       string                              `json:"exact_version"`
	OperatingSystem    string                              `json:"os"`
	Architecture       string                              `json:"arch"`
	Variant            string                              `json:"variant"`
	SourceKind         store.CoreArtifactSourceKind        `json:"source_kind"`
	UserSource         string                              `json:"user_source,omitempty"`
	RepositoryID       int64                               `json:"repository_id,omitempty"`
	ReleaseID          int64                               `json:"release_id,omitempty"`
	AssetID            int64                               `json:"asset_id,omitempty"`
	ArchiveSHA256      string                              `json:"archive_sha256"`
	BinarySHA256       string                              `json:"binary_sha256"`
	BinaryPath         string                              `json:"binary_path"`
	ReportedVersion    string                              `json:"reported_version"`
	FeatureFingerprint json.RawMessage                     `json:"feature_fingerprint"`
	VerificationState  store.CoreArtifactVerificationState `json:"verification_state"`
	CreatedAt          time.Time                           `json:"created_at"`
}

type CoreArtifactListFilter struct {
	ExactVersion      string
	Architecture      string
	Variant           string
	SourceKind        store.CoreArtifactSourceKind
	VerificationState store.CoreArtifactVerificationState
	Cursor            *CoreArtifactCursor
	Limit             int
}

type CoreArtifactCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type CoreArtifactPage struct {
	Items []CoreArtifact      `json:"items"`
	Next  *CoreArtifactCursor `json:"next,omitempty"`
}

type CoreImportRequest struct {
	SourcePath        string
	SourceDescription string
	SHA256            string
	ExactVersion      string
	Architecture      string
	Variant           string
	DeleteSource      bool
}

type coreInstallPayload struct {
	Asset catalog.Asset `json:"asset"`
}

type coreImportPayload struct {
	SourcePath        string `json:"source_path"`
	SourceDescription string `json:"source_description"`
	SHA256            string `json:"sha256"`
	ExactVersion      string `json:"exact_version"`
	Architecture      string `json:"architecture"`
	Variant           string `json:"variant"`
	DeleteSource      bool   `json:"delete_source,omitempty"`
}

type CatalogRefreshOptions struct {
	Force bool `json:"force"`
}

func (application *Application) RefreshCatalog(ctx context.Context, options CatalogRefreshOptions) (CatalogSnapshot, error) {
	if !options.Force && application.settings.GitHub.CatalogTTLHours > 0 {
		state, stateErr := application.database.CatalogState(ctx)
		if stateErr == nil && application.now().UTC().Before(
			state.RefreshedAt.Add(time.Duration(application.settings.GitHub.CatalogTTLHours)*time.Hour),
		) {
			snapshot, catalogErr := application.Catalog(ctx)
			if catalogErr != nil {
				return CatalogSnapshot{}, catalogErr
			}
			snapshot.NotModified = true
			snapshot.Diagnostics = append(snapshot.Diagnostics, catalog.Diagnostic{
				Step: catalog.StepReleases, Severity: catalog.DiagnosticInfo, Code: "ttl_fresh",
				Message: "the cached official catalog is still within its configured TTL",
			})
			return snapshot, nil
		}
		if stateErr != nil && !errors.Is(stateErr, store.ErrCatalogStateNotFound) {
			return CatalogSnapshot{}, stateErr
		}
	}
	client, err := catalog.NewGitHubClient(catalog.ClientOptions{Token: application.settings.GitHub.Token})
	if err != nil {
		return CatalogSnapshot{}, err
	}
	return application.refreshCatalogWith(ctx, client)
}

func (application *Application) refreshCatalogWith(ctx context.Context, client catalogRefresher) (CatalogSnapshot, error) {
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

func (application *Application) Catalog(ctx context.Context) (CatalogSnapshot, error) {
	state, err := application.database.CatalogState(ctx)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	decoded, err := decodeCatalog(state.Catalog)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	var diagnostics []catalog.Diagnostic
	if err := jsonstrict.Decode(state.Diagnostics, maximumCatalogBytes, &diagnostics); err != nil {
		return CatalogSnapshot{}, fmt.Errorf("decode catalog diagnostics: %w", err)
	}
	return CatalogSnapshot{
		Catalog: decoded, Validator: state.Validator, Diagnostics: diagnostics, RefreshedAt: state.RefreshedAt,
	}, nil
}

func (application *Application) ListCatalogAssets(
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
	assets := make([]catalog.Asset, 0)
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
		if filter.Installable {
			if _, err := asset.TrustedDigest(); err != nil {
				continue
			}
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

func (application *Application) QueueCatalogRefresh(ctx context.Context, options CatalogRefreshOptions) (Task, error) {
	payload, err := json.Marshal(options)
	if err != nil {
		return Task{}, err
	}
	return application.queueMaintenanceTask(ctx, store.TaskKindCatalogRefresh, payload, "")
}

func decodeCatalog(raw json.RawMessage) (catalog.Catalog, error) {
	var decoded catalog.Catalog
	if err := jsonstrict.Decode(raw, maximumCatalogBytes, &decoded); err != nil {
		return catalog.Catalog{}, fmt.Errorf("decode stored catalog: %w", err)
	}
	if err := validateCatalog(decoded); err != nil {
		return catalog.Catalog{}, err
	}
	return decoded, nil
}

func validateCatalog(value catalog.Catalog) error {
	if value.RepositoryID != catalog.OfficialRepositoryID {
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

func coreArtifact(value store.CoreArtifact) CoreArtifact {
	return CoreArtifact{
		ID: value.ID, ExactVersion: value.ExactVersion, OperatingSystem: value.OperatingSystem,
		Architecture: value.Architecture, Variant: value.Variant, SourceKind: value.SourceKind,
		UserSource: value.UserSource, RepositoryID: value.RepositoryID, ReleaseID: value.ReleaseID,
		AssetID: value.AssetID, ArchiveSHA256: value.ArchiveSHA256, BinarySHA256: value.BinarySHA256,
		BinaryPath:      value.BinaryPath,
		ReportedVersion: value.ReportedVersion, FeatureFingerprint: append(json.RawMessage(nil), value.FeatureFingerprint...),
		VerificationState: value.VerificationState, CreatedAt: value.CreatedAt,
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

func IsNoRunningCore(err error) bool {
	return errors.Is(err, ErrNoRunningCore)
}
