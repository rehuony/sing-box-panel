package store

import "context"

// saveTestConfiguration creates a saved-file fixture and returns its runtime snapshot.
func saveTestConfiguration(ctx context.Context, database *Store, expected int64, revision NewCanonicalRevision) (CanonicalRevision, error) {
	file, err := database.SaveConfigurationFile(ctx, expected, string(revision.Document), revision)
	if err != nil {
		return CanonicalRevision{}, err
	}
	return database.GetCanonicalRevision(ctx, file.CanonicalRevisionID)
}
