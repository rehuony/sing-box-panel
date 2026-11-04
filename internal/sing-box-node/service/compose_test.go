// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// newApplication returns a node over database whose data directory tests
// replace when they write files.
func newApplication(database *store.Store) *Node {
	return &Node{
		database: database, dataDir: filepath.Join(os.TempDir(), "sing-box-node-test"),
		now: time.Now, random: rand.Read, removeFile: os.Remove,
		runtime: NewRuntimeIdentityResolver(database), publicIP: host.NewPublicIPDetector(),
	}
}

func FromStore(database *store.Store) *Node { return newApplication(database) }

func FromStoreWithRuntimeResolver(database *store.Store, resolver RuntimeResolver) *Node {
	node := newApplication(database)
	node.runtime = resolver
	return node
}

// receive records content as a configuration the panel sent, under a
// revision ID derived from it.
func receive(ctx context.Context, node *Node, content string) (store.CanonicalRevision, error) {
	sum := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(sum[:])
	return node.ReceiveConfiguration(ctx, "rev-"+digest[:16], []byte(content), digest)
}

// recordRevision records revision as a configuration the panel sent.
func recordRevision(ctx context.Context, database *store.Store, revision store.NewCanonicalRevision) (store.CanonicalRevision, error) {
	return database.RecordRevision(ctx, revision.ID, revision.Document, revision.CreatedAt)
}
