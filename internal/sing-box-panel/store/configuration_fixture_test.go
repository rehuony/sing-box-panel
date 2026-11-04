package store

import (
	"context"
	"time"
)

// testNodeID names the node that openTestStore registers.
const testNodeID = "node_test"

// saveTestConfiguration creates a saved-file fixture for the test node and
// returns its runtime snapshot.
func saveTestConfiguration(ctx context.Context, database *Store, expected int64, revision NewCanonicalRevision) (CanonicalRevision, error) {
	file, err := database.SaveConfigurationFile(ctx, testNodeID, expected, string(revision.Document), revision)
	if err != nil {
		return CanonicalRevision{}, err
	}
	return database.GetCanonicalRevision(ctx, file.CanonicalRevisionID)
}

func createTestNode(ctx context.Context, database *Store) error {
	now := time.Now().UTC()
	_, err := database.CreateNode(ctx, NewNode{ID: testNodeID, Name: "test", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	return err
}
