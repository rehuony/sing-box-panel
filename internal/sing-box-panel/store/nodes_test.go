// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestNodesExistOnlyOnceTheyEnroll(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Now().UTC()
	issue := func(token byte, id, name string) {
		t.Helper()
		if err := database.IssueNewNodeEnrollment(ctx, NewNodeEnrollment{
			TokenHash: [32]byte{token}, NodeID: id, Name: name, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	enroll := func(token byte, key byte, initial *ConfigurationFileUpdate) (Node, bool, error) {
		return database.EnrollNode(ctx, [32]byte{token}, NodeEnrollment{
			PublicKey: bytes.Repeat([]byte{key}, 32), Hostname: "host", EnrolledAt: now, Configuration: initial,
		})
	}

	issue(1, "node_edge", "edge")
	if nodes, err := database.Nodes(ctx); err != nil || len(nodes) != 1 || nodes[0].ID != testNodeID {
		t.Fatalf("nodes with an unused code = %+v, %v; want only the test node", nodes, err)
	}
	if _, err := database.NodeByName(ctx, "edge"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("an unused code's name found a node: %v", err)
	}
	// A new code for the same name, in any case, replaces the unused one.
	issue(2, "node_edge_2", "EDGE")
	if _, _, err := enroll(1, 1, nil); !errors.Is(err, ErrEnrollmentInvalid) {
		t.Fatalf("a replaced code = %v, want ErrEnrollmentInvalid", err)
	}
	initial, err := NewConfigurationFileUpdate(rand.Read, now, 0, "{}")
	if err != nil {
		t.Fatal(err)
	}
	node, created, err := enroll(2, 2, initial)
	if err != nil || !created || node.ID != "node_edge_2" || node.Name != "EDGE" || node.Hostname != "host" || !node.EnrolledAt.Equal(now) {
		t.Fatalf("enrolling a new node's code = %+v, %t, %v", node, created, err)
	}
	if file, err := database.ConfigurationFile(ctx, node.ID); err != nil || file.Revision != 1 || file.Content != "{}" {
		t.Fatalf("first configuration = %+v, %v", file, err)
	}
	if _, _, err := enroll(2, 2, nil); !errors.Is(err, ErrEnrollmentInvalid) {
		t.Fatalf("a used code = %v, want ErrEnrollmentInvalid", err)
	}
	if err := database.IssueNewNodeEnrollment(ctx, NewNodeEnrollment{
		TokenHash: [32]byte{3}, NodeID: "node_other", Name: "edge", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); !errors.Is(err, ErrNodeNameConflict) {
		t.Fatalf("a code for an enrolled node's name = %v, want ErrNodeNameConflict", err)
	}

	// A canceled code and an expired one create nothing.
	issue(4, "node_canceled", "canceled")
	if canceled, err := database.CancelNodeEnrollment(ctx, "node_canceled"); err != nil || !canceled {
		t.Fatalf("canceling an unused code = %t, %v", canceled, err)
	}
	if canceled, err := database.CancelNodeEnrollment(ctx, "node_canceled"); err != nil || canceled {
		t.Fatalf("canceling it again = %t, %v; want nothing canceled", canceled, err)
	}
	if _, _, err := enroll(4, 4, nil); !errors.Is(err, ErrEnrollmentInvalid) {
		t.Fatalf("a canceled code = %v, want ErrEnrollmentInvalid", err)
	}
	if err := database.IssueNewNodeEnrollment(ctx, NewNodeEnrollment{
		TokenHash: [32]byte{5}, NodeID: "node_expired", Name: "expired", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := enroll(5, 5, nil); !errors.Is(err, ErrEnrollmentInvalid) {
		t.Fatalf("an expired code = %v, want ErrEnrollmentInvalid", err)
	}

	// A node that took the code's name since it was issued fails the
	// enrollment, which keeps the code.
	issue(6, "node_late", "late")
	if _, err := database.RenameNode(ctx, testNodeID, "late"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := enroll(6, 6, nil); !errors.Is(err, ErrNodeNameConflict) {
		t.Fatalf("enrolling under a taken name = %v, want ErrNodeNameConflict", err)
	}
	if _, err := database.Node(ctx, "node_late"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("a failed enrollment created a node: %v", err)
	}
	if _, err := database.RenameNode(ctx, testNodeID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, created, err := enroll(6, 6, nil); err != nil || !created {
		t.Fatalf("the kept code = %t, %v", created, err)
	}

	// An enrolled node's code replaces its key and keeps the node; canceling
	// it keeps the current key.
	if _, err := database.IssueNodeEnrollment(ctx, node.ID, [32]byte{7}, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CancelNodeEnrollment(ctx, node.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := enroll(7, 7, nil); !errors.Is(err, ErrEnrollmentInvalid) {
		t.Fatalf("a canceled code of an enrolled node = %v, want ErrEnrollmentInvalid", err)
	}
	if _, err := database.IssueNodeEnrollment(ctx, node.ID, [32]byte{8}, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	enrolled, created, err := enroll(8, 8, nil)
	if err != nil || created || enrolled.ID != node.ID || enrolled.Name != node.Name || !bytes.Equal(enrolled.PublicKey, bytes.Repeat([]byte{8}, 32)) {
		t.Fatalf("enrolling an enrolled node again = %+v, %t, %v", enrolled, created, err)
	}
	if _, err := database.IssueNodeEnrollment(ctx, "node_missing", [32]byte{9}, now, now.Add(time.Hour)); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("a code for a missing node = %v, want ErrNodeNotFound", err)
	}
}

func TestEnrollingANewNodeSavesItsFirstConfigurationOrNothing(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Now().UTC()
	initial, err := NewConfigurationFileUpdate(rand.Read, now, 0, "{}")
	if err != nil {
		t.Fatal(err)
	}
	invalid := &ConfigurationFileUpdate{ExpectedRevision: 0, Content: "x\x00", Revision: initial.Revision}
	if _, err := enrollTestNode(ctx, database, "node_other", "other", invalid); !errors.Is(err, ErrConfigurationFileInvalid) {
		t.Fatalf("invalid first configuration = %v, want ErrConfigurationFileInvalid", err)
	}
	if _, err := database.Node(ctx, "node_other"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("a node whose configuration failed was kept: %v", err)
	}
}
