package testutil

import (
	"context"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// SaveConfiguration creates a saved-file fixture and returns its runtime snapshot.
func SaveConfiguration(ctx context.Context, database *store.Store, expected int64, revision store.NewCanonicalRevision) (store.CanonicalRevision, error) {
	file, err := database.SaveConfigurationFile(ctx, expected, string(revision.Document), revision)
	if err != nil {
		return store.CanonicalRevision{}, err
	}
	return database.GetCanonicalRevision(ctx, file.CanonicalRevisionID)
}
