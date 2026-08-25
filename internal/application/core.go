// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
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

type CapabilityStatus struct {
	Resolution   CoreVersionResolution   `json:"resolution"`
	SupportLevel capability.SupportLevel `json:"support_level"`
	Pinned       bool                    `json:"pinned"`
	Pin          *store.CapabilityPin    `json:"pin,omitempty"`
	Quarantined  bool                    `json:"quarantined"`
	ReasonCode   string                  `json:"reason_code,omitempty"`
	Presentation *CapabilityPresentation `json:"presentation,omitempty"`
}

// CapabilityPresentation is the inert, validated subset of an exact pinned
// manifest that transports may expose to built-in editors. It deliberately
// contains no transforms, executable components, templates, or remote
// resources.
type CapabilityPresentation struct {
	SemanticFacts []capability.SemanticFact    `json:"semantic_facts"`
	UI            []CapabilityUIDescriptorView `json:"ui"`
}

type CapabilityUIDescriptorView struct {
	ID          string                             `json:"id"`
	FactID      string                             `json:"fact_id"`
	Kind        capability.UIKind                  `json:"kind"`
	Label       string                             `json:"label"`
	Help        string                             `json:"help,omitempty"`
	Order       int                                `json:"order,omitempty"`
	Options     []capability.UIOption              `json:"options,omitempty"`
	VisibleWhen *CapabilityVisibilityConditionView `json:"visible_when,omitempty"`
}

// CapabilityVisibilityConditionView transports the exact canonical JSON
// spelling of the validated scalar. Encoding it as a string prevents a browser
// JSON parser from rounding an integer before comparing visibility rules.
type CapabilityVisibilityConditionView struct {
	CanonicalPath string `json:"canonical_path"`
	EqualsJSON    string `json:"equals_json"`
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
}

func (application *Application) RefreshCatalog(ctx context.Context) (CatalogSnapshot, error) {
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

func (application *Application) QueueCatalogRefresh(ctx context.Context) (Task, error) {
	return application.queueMaintenanceTask(ctx, "catalog-refresh", json.RawMessage(`{}`), "")
}

func (application *Application) CoreCapabilityStatus(
	ctx context.Context,
	explicitVersion string,
) (CapabilityStatus, error) {
	resolution, err := application.ResolveCoreVersion(ctx, explicitVersion)
	if err != nil {
		return CapabilityStatus{}, err
	}
	result := CapabilityStatus{
		Resolution:   resolution,
		SupportLevel: capability.SupportManualJSON,
	}
	snapshot, err := application.database.PinnedCapability(ctx, resolution.ExactVersion)
	if errors.Is(err, store.ErrCapabilityPinNotFound) {
		return result, nil
	}
	if err != nil {
		return CapabilityStatus{}, err
	}
	result.Pinned = true
	pin := snapshot.Pin
	result.Pin = &pin
	result.SupportLevel = snapshot.Pin.SupportLevel
	if snapshot.Quarantine == nil {
		if snapshot.Pin.SupportLevel == capability.SupportNativeStructured ||
			snapshot.Pin.SupportLevel == capability.SupportCompatibleStructured {
			manifest, manifestErr := decodeStoredCapabilityManifest(snapshot.Manifest)
			if manifestErr != nil {
				return CapabilityStatus{}, manifestErr
			}
			presentation, presentationErr := newCapabilityPresentation(manifest)
			if presentationErr != nil {
				return CapabilityStatus{}, presentationErr
			}
			result.Presentation = presentation
		}
		return result, nil
	}
	result.Quarantined = true
	result.ReasonCode = snapshot.Quarantine.ReasonCode
	// A quarantined manifest is retained for audit and rollback evidence but is
	// never effective for new projection work. The safe exact-version fallback
	// is manual JSON, not another version's manifest.
	result.SupportLevel = capability.SupportManualJSON
	return result, nil
}

func newCapabilityPresentation(manifest *capability.Manifest) (*CapabilityPresentation, error) {
	descriptors := manifest.UIDescriptors()
	ui := make([]CapabilityUIDescriptorView, len(descriptors))
	for index, descriptor := range descriptors {
		ui[index] = CapabilityUIDescriptorView{
			ID:      descriptor.ID,
			FactID:  descriptor.FactID,
			Kind:    descriptor.Kind,
			Label:   descriptor.Label,
			Help:    descriptor.Help,
			Order:   descriptor.Order,
			Options: append([]capability.UIOption(nil), descriptor.Options...),
		}
		if descriptor.VisibleWhen != nil {
			equals, err := json.Marshal(descriptor.VisibleWhen.Equals)
			if err != nil {
				return nil, fmt.Errorf("encode capability UI visibility condition: %w", err)
			}
			ui[index].VisibleWhen = &CapabilityVisibilityConditionView{
				CanonicalPath: descriptor.VisibleWhen.CanonicalPath,
				EqualsJSON:    string(equals),
			}
		}
	}
	return &CapabilityPresentation{
		SemanticFacts: manifest.SemanticFacts(),
		UI:            ui,
	}, nil
}

func (application *Application) QueueCoreInstall(ctx context.Context, assetID int64) (Task, error) {
	asset, err := application.catalogAsset(ctx, assetID)
	if err != nil {
		return Task{}, err
	}
	digest, err := asset.TrustedDigest()
	if err != nil {
		return Task{}, err
	}
	payload, err := json.Marshal(coreInstallPayload{Asset: asset})
	if err != nil {
		return Task{}, err
	}
	key := fmt.Sprintf("core-install:%d:%d:%d:%s", asset.RepositoryID, asset.ReleaseID, asset.AssetID, digest.String())
	return application.queueMaintenanceTask(ctx, "core-install", payload, key)
}

func (application *Application) QueueCoreImport(ctx context.Context, request CoreImportRequest) (Task, error) {
	if !filepath.IsAbs(request.SourcePath) || filepath.Clean(request.SourcePath) != request.SourcePath {
		return Task{}, errors.New("core import path must be absolute and clean")
	}
	digest, err := coreartifact.ParseSHA256(request.SHA256)
	if err != nil || digest.IsZero() {
		return Task{}, errors.New("core import SHA-256 is invalid")
	}
	version, err := coreartifact.ParseExactVersion(request.ExactVersion)
	if err != nil || version.IsZero() {
		return Task{}, errors.New("core import exact version is invalid")
	}
	architecture := coreartifact.Architecture(request.Architecture)
	if architecture != coreartifact.ArchitectureAMD64 && architecture != coreartifact.ArchitectureARM64 {
		return Task{}, errors.New("core import architecture must be amd64 or arm64")
	}
	variant := coreartifact.Variant(request.Variant)
	if variant == "" {
		variant = coreartifact.VariantPlain
	}
	source, err := coreartifact.NewUserSource(request.SourceDescription)
	if err != nil {
		return Task{}, err
	}
	if _, err := coreartifact.NewIdentity(
		source, digest, coreartifact.OperatingSystemLinux,
		architecture, variant, version,
	); err != nil {
		return Task{}, err
	}
	payload, err := json.Marshal(coreImportPayload{
		SourcePath: request.SourcePath, SourceDescription: request.SourceDescription,
		SHA256: digest.String(), ExactVersion: version.String(), Architecture: string(architecture), Variant: string(variant),
	})
	if err != nil {
		return Task{}, err
	}
	key := "core-import:" + digest.String() + ":" + version.String() + ":" + string(architecture) + ":" + string(variant)
	return application.queueMaintenanceTask(ctx, "core-import", payload, key)
}

func (application *Application) queueMaintenanceTask(
	ctx context.Context,
	kind string,
	payload json.RawMessage,
	idempotencyKey string,
) (Task, error) {
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	queued, err := application.database.EnqueueTask(ctx, store.EnqueueTaskInput{
		ID: taskID, IdempotencyKey: idempotencyKey, Lane: store.TaskLaneMaintenance,
		Kind: kind, Payload: payload, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(queued), nil
}

func (application *Application) catalogAsset(ctx context.Context, assetID int64) (catalog.Asset, error) {
	if assetID <= 0 {
		return catalog.Asset{}, errors.New("catalog asset id must be positive")
	}
	snapshot, err := application.Catalog(ctx)
	if err != nil {
		return catalog.Asset{}, err
	}
	for _, asset := range snapshot.Catalog.Assets() {
		if asset.AssetID == assetID {
			return asset, nil
		}
	}
	return catalog.Asset{}, errors.New("catalog asset not found")
}

func (application *Application) ListCoreArtifacts(
	ctx context.Context,
	filter CoreArtifactListFilter,
) (CoreArtifactPage, error) {
	var cursor *store.CreatedAtCursor
	if filter.Cursor != nil {
		cursor = &store.CreatedAtCursor{
			CreatedAt: filter.Cursor.CreatedAt,
			ID:        strings.TrimSpace(filter.Cursor.ID),
		}
	}
	page, err := application.database.ListCoreArtifacts(ctx, store.CoreArtifactListFilter{
		ExactVersion: filter.ExactVersion, Architecture: filter.Architecture, Variant: filter.Variant,
		SourceKind: filter.SourceKind, VerificationState: filter.VerificationState,
		Cursor: cursor, Limit: filter.Limit,
	})
	if err != nil {
		return CoreArtifactPage{}, err
	}
	result := CoreArtifactPage{Items: make([]CoreArtifact, len(page.Items))}
	for index, artifact := range page.Items {
		result.Items[index] = coreArtifact(artifact)
	}
	if page.Next != nil {
		result.Next = &CoreArtifactCursor{CreatedAt: page.Next.CreatedAt, ID: page.Next.ID}
	}
	return result, nil
}

func (application *Application) CoreArtifact(ctx context.Context, artifactID string) (CoreArtifact, error) {
	artifact, err := application.database.GetCoreArtifact(ctx, artifactID)
	if err != nil {
		return CoreArtifact{}, err
	}
	return coreArtifact(artifact), nil
}

func (application *Application) RestrictCoreArtifactVerification(
	ctx context.Context,
	artifactID string,
	verificationState store.CoreArtifactVerificationState,
) (CoreArtifact, error) {
	artifact, err := application.database.RestrictCoreArtifactVerification(
		ctx,
		artifactID,
		verificationState,
		application.now().UTC(),
	)
	if err != nil {
		return CoreArtifact{}, err
	}
	return coreArtifact(artifact), nil
}

func (application *Application) RemoveCoreArtifact(ctx context.Context, artifactID string) error {
	return application.database.RemoveCoreArtifact(ctx, artifactID)
}

func (application *Application) PersistInstalledCore(
	ctx context.Context,
	result artifactstore.Result,
) (CoreArtifact, error) {
	featureFingerprint, err := result.FeatureFingerprint.CanonicalJSON()
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("normalize installed core feature fingerprint: %w", err)
	}
	identityJSON, err := json.Marshal(result.Identity)
	if err != nil {
		return CoreArtifact{}, err
	}
	id := sha256.Sum256(identityJSON)
	source := result.Identity.Source()
	persisted := store.CoreArtifact{
		ID: "core_" + hex.EncodeToString(id[:]), ExactVersion: result.Identity.ReportedVersion().String(),
		OperatingSystem: string(result.Identity.OperatingSystem()), Architecture: string(result.Identity.Architecture()),
		Variant: string(result.Identity.Variant()), ArchiveSHA256: result.Identity.Digest().String(),
		BinarySHA256: result.BinarySHA256.String(),
		BinaryPath:   result.BinaryPath, ReportedVersion: result.Identity.ReportedVersion().String(),
		FeatureFingerprint: featureFingerprint, VerificationState: store.CoreArtifactVerified,
		CreatedAt: application.now().UTC(),
	}
	switch source.Kind() {
	case coreartifact.SourceOfficial:
		persisted.SourceKind = store.CoreArtifactSourceOfficial
		persisted.RepositoryID = source.RepositoryID()
		persisted.ReleaseID = source.ReleaseID()
		persisted.AssetID = source.AssetID()
	case coreartifact.SourceUser:
		persisted.SourceKind = store.CoreArtifactSourceUserVerified
		persisted.UserSource = source.UserSource()
	default:
		return CoreArtifact{}, errors.New("installed core has an unknown source kind")
	}
	stored, err := application.database.UpsertCoreArtifact(ctx, persisted)
	if err != nil {
		return CoreArtifact{}, err
	}
	if stored.VerificationState != store.CoreArtifactVerified {
		return CoreArtifact{}, fmt.Errorf(
			"%w: %s remains %s",
			ErrCoreArtifactVerificationBlocked,
			stored.ID,
			stored.VerificationState,
		)
	}
	return coreArtifact(stored), nil
}

// ExecuteCoreArtifactTask performs the slow verified install/import outside a
// database transaction, then crosses one explicit safe cancellation boundary
// before publishing the immutable identity into SQLite.
func (application *Application) ExecuteCoreArtifactTask(
	ctx context.Context,
	kind string,
	payload json.RawMessage,
	installer ArtifactInstaller,
	beforePersist func(context.Context) error,
) (CoreArtifact, error) {
	if installer == nil {
		return CoreArtifact{}, errors.New("artifact installer is unavailable")
	}
	var result artifactstore.Result
	switch kind {
	case "core-install":
		var input coreInstallPayload
		if err := jsonstrict.Decode(payload, 128<<10, &input); err != nil {
			return CoreArtifact{}, fmt.Errorf("decode core install task: %w", err)
		}
		if err := input.Asset.Validate(); err != nil {
			return CoreArtifact{}, err
		}
		if _, err := input.Asset.TrustedDigest(); err != nil {
			return CoreArtifact{}, err
		}
		installed, err := installer.InstallOfficial(ctx, input.Asset)
		if err != nil {
			return CoreArtifact{}, err
		}
		result = installed
	case "core-import":
		var input coreImportPayload
		if err := jsonstrict.Decode(payload, 128<<10, &input); err != nil {
			return CoreArtifact{}, fmt.Errorf("decode core import task: %w", err)
		}
		digest, err := coreartifact.ParseSHA256(input.SHA256)
		if err != nil || digest.IsZero() {
			return CoreArtifact{}, errors.New("core import task digest is invalid")
		}
		version, err := coreartifact.ParseExactVersion(input.ExactVersion)
		if err != nil || version.IsZero() {
			return CoreArtifact{}, errors.New("core import task version is invalid")
		}
		imported, err := installer.ImportLocal(ctx, artifactstore.ImportRequest{
			SourcePath: input.SourcePath, SourceDescription: input.SourceDescription,
			ExpectedSHA256: digest, ExpectedVersion: version,
			ExpectedArchitecture: coreartifact.Architecture(input.Architecture), Variant: coreartifact.Variant(input.Variant),
		})
		if err != nil {
			return CoreArtifact{}, err
		}
		result = imported
	default:
		return CoreArtifact{}, fmt.Errorf("unsupported core artifact task %q", kind)
	}
	if beforePersist != nil {
		if err := beforePersist(ctx); err != nil {
			return CoreArtifact{}, err
		}
	}
	return application.PersistInstalledCore(ctx, result)
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
	return errors.Is(err, runtimeidentity.ErrNoRunningCore)
}
