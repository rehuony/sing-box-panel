// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Failures of installing one exact version on many nodes.
var (
	// ErrVersionNotInCatalog refuses a version the catalog has no official
	// archive of, before any node is commanded.
	ErrVersionNotInCatalog = errors.New("the catalog has no official archive of this version")
	// ErrNodePlatformUnknown fails a node that has not reported its platform.
	ErrNodePlatformUnknown = errors.New("the node has not reported its platform")
	// ErrCoreAssetUnavailable fails a node whose platform the version has
	// no official archive for.
	ErrCoreAssetUnavailable = errors.New("the version has no official archive for the node's platform")
	// ErrCoreDownloadFailed fails the nodes of a platform whose archive the
	// panel could not download and verify.
	ErrCoreDownloadFailed = errors.New("the panel could not download and verify the release asset")
)

// platformArchive is the official archive of one platform in a batch
// install, cached once, when the first node of that platform needs it.
type platformArchive struct {
	assetID int64
	once    sync.Once
	cached  store.CachedArtifact
	err     error
}

// VersionInstaller returns a command that installs exactVersion on a node
// from the official catalog archive for the node's own platform. The catalog
// must hold an archive of the version; an uninitialized catalog fails as
// corecache reports it.
func (service *Service) VersionInstaller(ctx context.Context, exactVersion string) (func(context.Context, Node) error, error) {
	assets, err := service.cores.ListCatalogAssets(ctx, corecache.CatalogAssetFilter{ExactVersion: exactVersion})
	if err != nil {
		return nil, err
	}
	if len(assets.Assets) == 0 {
		return nil, ErrVersionNotInCatalog
	}
	archives := map[string]*platformArchive{}
	for _, asset := range assets.Assets {
		platform := string(asset.OperatingSystem) + "/" + string(asset.Architecture)
		if archives[platform] == nil {
			archives[platform] = &platformArchive{assetID: asset.AssetID}
		}
	}
	return func(ctx context.Context, node Node) error {
		if node.OS == "" || node.Arch == "" {
			return ErrNodePlatformUnknown
		}
		archive := archives[node.OS+"/"+node.Arch]
		if archive == nil {
			return ErrCoreAssetUnavailable
		}
		archive.once.Do(func() { archive.cached, archive.err = service.cores.CacheOfficial(ctx, archive.assetID) })
		if archive.err != nil {
			return ErrCoreDownloadFailed
		}
		_, err := service.InstallCore(ctx, node.ID, archive.cached)
		return err
	}, nil
}
