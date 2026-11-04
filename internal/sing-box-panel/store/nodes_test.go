// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestCreateNodeSavesItsFirstConfigurationOrNothing(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Now().UTC()
	initial, err := NewConfigurationFileUpdate(rand.Read, now, 0, "{}")
	if err != nil {
		t.Fatal(err)
	}
	node, err := database.CreateNode(ctx, NewNode{ID: "node_edge", Name: "edge", CreatedAt: now, TokenHash: [32]byte{1}, ExpiresAt: now.Add(time.Hour), Configuration: initial})
	if err != nil {
		t.Fatal(err)
	}
	file, err := database.ConfigurationFile(ctx, node.ID)
	if err != nil || file.Revision != 1 || file.Content != "{}" || file.CanonicalRevisionID == "" {
		t.Fatalf("first configuration = %+v, %v", file, err)
	}
	head, err := database.Head(ctx, node.ID)
	if err != nil || head == nil || head.ID != file.CanonicalRevisionID || head.Sequence != 1 {
		t.Fatalf("first revision = %+v, %v", head, err)
	}

	invalid := &ConfigurationFileUpdate{ExpectedRevision: 0, Content: "x\x00", Revision: initial.Revision}
	if _, err := database.CreateNode(ctx, NewNode{ID: "node_other", Name: "other", CreatedAt: now, TokenHash: [32]byte{2}, ExpiresAt: now.Add(time.Hour), Configuration: invalid}); !errors.Is(err, ErrConfigurationFileInvalid) {
		t.Fatalf("invalid first configuration = %v, want ErrConfigurationFileInvalid", err)
	}
	if _, err := database.Node(ctx, "node_other"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("a node whose configuration failed was kept: %v", err)
	}
}
