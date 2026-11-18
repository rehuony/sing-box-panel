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

// CreateNode enrolls a node named name in database, with no session and no
// configuration file, and returns its ID.
func CreateNode(t testing.TB, database *store.Store, name string) string {
	t.Helper()
	id, _ := EnrollNode(t, database, name, store.NodeEnrollment{})
	return id
}

// EnrollNode enrolls a node named name in database as a host presenting
// input would, with a new key when input has none, and returns its ID and
// key.
func EnrollNode(t testing.TB, database *store.Store, name string, input store.NodeEnrollment) (string, []byte) {
	t.Helper()
	ctx, now := t.Context(), time.Now().UTC()
	if input.EnrolledAt.IsZero() {
		input.EnrolledAt = now
	}
	if input.PublicKey == nil {
		input.PublicKey = make([]byte, ed25519.PublicKeySize)
		if _, err := rand.Read(input.PublicKey); err != nil {
			t.Fatal(err)
		}
	}
	token := sha256.Sum256([]byte("token-" + name))
	if err := database.IssueNewNodeEnrollment(ctx, store.NewNodeEnrollment{
		TokenHash: token, NodeID: "node_" + name, Name: name, CreatedAt: input.EnrolledAt, ExpiresAt: input.EnrolledAt.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	node, _, err := database.EnrollNode(ctx, token, input)
	if err != nil {
		t.Fatal(err)
	}
	return node.ID, input.PublicKey
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
	id, key := EnrollNode(t, database, name, store.NodeEnrollment{})
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
