// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// CreateNode registers a node named name in database and returns its ID.
func CreateNode(t testing.TB, database *store.Store, name string) string {
	t.Helper()
	now := time.Now().UTC()
	node, err := database.CreateNode(t.Context(), store.NewNode{
		ID: "node_" + name, Name: name, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		TokenHash: sha256.Sum256([]byte("token-" + name)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return node.ID
}
