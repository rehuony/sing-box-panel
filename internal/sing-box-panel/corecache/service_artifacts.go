// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"errors"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
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

func (application *Service) ListCoreArtifacts(ctx context.Context, filter CoreArtifactListFilter) (CoreArtifactPage, error) {
	var cursor *store.CreatedAtCursor
	if filter.Cursor != nil {
		cursor = &store.CreatedAtCursor{CreatedAt: filter.Cursor.CreatedAt, ID: strings.TrimSpace(filter.Cursor.ID)}
	}
	page, err := application.database.ListCoreArtifacts(ctx, store.CoreArtifactListFilter{
		ExactVersion: filter.ExactVersion, Architecture: filter.Architecture, Variant: filter.Variant,
		SourceKind: filter.SourceKind,
		Cursor:     cursor, Limit: filter.Limit,
	})
	if err != nil {
		return CoreArtifactPage{}, err
	}
	result := CoreArtifactPage{Items: make([]CoreArtifact, len(page.Items))}
	for index, artifact := range page.Items {
		result.Items[index] = CoreArtifactFromStore(artifact)
	}
	if page.Next != nil {
		result.Next = &CoreArtifactCursor{CreatedAt: page.Next.CreatedAt, ID: page.Next.ID}
	}
	return result, nil
}

func (application *Service) CoreArtifact(ctx context.Context, artifactID string) (CoreArtifact, error) {
	artifact, err := application.database.GetCoreArtifact(ctx, artifactID)
	if err != nil {
		return CoreArtifact{}, err
	}
	return CoreArtifactFromStore(artifact), nil
}
