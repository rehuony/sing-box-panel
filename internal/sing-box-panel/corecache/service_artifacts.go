// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"errors"
)

// CatalogAsset returns one asset of the stored catalog.
func (application *Service) CatalogAsset(ctx context.Context, assetID int64) (Asset, error) {
	if assetID <= 0 {
		return Asset{}, errors.New("catalog asset id must be positive")
	}
	snapshot, err := application.Catalog(ctx)
	if err != nil {
		return Asset{}, err
	}
	for _, asset := range snapshot.Catalog.Assets() {
		if asset.AssetID == assetID {
			return asset, nil
		}
	}
	return Asset{}, errors.New("catalog asset not found")
}
