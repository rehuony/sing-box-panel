// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
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

// InboundDocument is a configuration with one inbound that subscriptions
// publish.
const InboundDocument = `{"inbounds":[{"type":"vless","tag":"vless-in","listen":"::","listen_port":443,"users":[{"name":"one","uuid":"uuid"}]}]}`

// PublishNode registers a node named name that runs document with core
// 1.13.19, as its state report would record it, and returns its ID. The node
// publishes at publicHost, or at reportedIP without one.
func PublishNode(t testing.TB, database *store.Store, name, document, publicHost, reportedIP string) string {
	t.Helper()
	ctx, now := t.Context(), time.Now().UTC()
	id := CreateNode(t, database, name)
	update, err := store.NewConfigurationFileUpdate(rand.Read, now, 0, document)
	if err != nil {
		t.Fatal(err)
	}
	file, err := database.SaveConfigurationFile(ctx, id, update.ExpectedRevision, update.Content, update.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveNodeSettings(ctx, id, 1, store.NodeSettings{
		PublicHost: publicHost, Traffic: store.NodeTrafficSettings{PeriodMonths: 1, ResetDay: 1},
	}); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, ed25519.PublicKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if _, err := database.EnrollNode(ctx, sha256.Sum256([]byte("token-"+name)), store.NodeEnrollment{PublicKey: key, EnrolledAt: now}); err != nil {
		t.Fatal(err)
	}
	session := "session_" + name
	if err := database.RecordNodeSession(ctx, id, key, store.NodeSession{ID: session, InstanceID: "instance_" + name, SeenAt: now}); err != nil {
		t.Fatal(err)
	}
	parsed, err := configuration.Parse([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(parsed.CanonicalJSON())
	if err := database.RecordNodeStateReport(ctx, id, session, []byte{}, store.NodeReport{
		PublicIP: reportedIP,
		Applied: store.AppliedBundle{
			ID: "bundle_" + name, RevisionID: file.CanonicalRevisionID, ConfigSHA256: hex.EncodeToString(sum[:]), ExactCoreVersion: "1.13.19",
		},
	}, now); err != nil {
		t.Fatal(err)
	}
	return id
}
