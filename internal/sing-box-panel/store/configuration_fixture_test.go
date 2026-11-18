package store

import (
	"context"
	"crypto/sha256"
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
	_, err := enrollTestNode(ctx, database, testNodeID, "test", nil)
	return err
}

// enrollTestNode creates node id named name as a host enrolling with a new
// node's code would, with initial as its first configuration file.
func enrollTestNode(ctx context.Context, database *Store, id, name string, initial *ConfigurationFileUpdate) (Node, error) {
	now := time.Now().UTC()
	token := sha256.Sum256([]byte("token-" + id))
	if err := database.IssueNewNodeEnrollment(ctx, NewNodeEnrollment{TokenHash: token, NodeID: id, Name: name, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		return Node{}, err
	}
	node, _, err := database.EnrollNode(ctx, token, NodeEnrollment{PublicKey: make([]byte, 32), EnrolledAt: now, Configuration: initial})
	return node, err
}
