// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/schema"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// TestConfigurationSchemaFollowsTheReleaseLine resolves schemas with a fake
// node: the 1.13 line has the reviewed schema, a later line asks the node
// once per archive, and a version below every line has none.
func TestConfigurationSchemaFollowsTheReleaseLine(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	for id, core := range map[string]struct{ version, digest string }{
		"core_reviewed": {"1.13.22", strings.Repeat("1", 64)},
		"core_native":   {"1.14.4", strings.Repeat("2", 64)},
		"core_upload":   {"1.14.4", strings.Repeat("2", 64)},
		"core_other":    {"1.15.0", strings.Repeat("3", 64)},
		"core_older":    {"1.13.18", strings.Repeat("4", 64)},
	} {
		if _, err := database.RecordCachedArtifact(ctx, store.CachedArtifact{
			ID: id, SourceKind: store.CachedArtifactUserVerified, UserSource: "fixture " + id, SHA256: core.digest, Size: 1,
			OS: "linux", Arch: "amd64", Variant: "musl", ExactVersion: core.version, CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	offline := errors.New("node offline")
	var asked []string
	configs := New(database, corecache.New(database, nil, nil, corecache.Options{}), func(_ context.Context, nodeID, coreArtifactID string) ([]byte, error) {
		asked = append(asked, coreArtifactID)
		if coreArtifactID == "core_other" {
			return nil, offline
		}
		return []byte(`{"type":"object","properties":{"log":{"type":"object","properties":{"level":{"enum":["warn"]}}}}}`), nil
	})

	reviewed, err := schema.Reviewed113()
	if err != nil {
		t.Fatal(err)
	}
	if contract, err := configs.ConfigurationSchema(ctx, "node", "core_reviewed"); err != nil || contract.SchemaSHA256 != reviewed.SHA256() {
		t.Fatalf("1.13 schema = %v, %v; want the reviewed schema", contract.SchemaSHA256, err)
	}
	for _, id := range []string{"core_native", "core_native", "core_upload"} {
		if contract, err := configs.ConfigurationSchema(ctx, "node", id); err != nil || contract.ExactVersion != "1.14.4" || !strings.Contains(string(contract.Schema), `"x-panel"`) {
			t.Fatalf("schema of %s = %+v, %v", id, contract, err)
		}
	}
	if _, err := configs.ConfigurationSchema(ctx, "node", "core_other"); !errors.Is(err, offline) {
		t.Fatalf("schema from an offline node error = %v", err)
	}
	if _, err := configs.ConfigurationSchema(ctx, "node", "core_older"); !errors.Is(err, schema.ErrConfigurationSchemaUnavailable) {
		t.Fatalf("schema below every line error = %v", err)
	}
	if strings.Join(asked, ",") != "core_native,core_other" {
		t.Fatalf("the node was asked for %v; want one fetch per archive", asked)
	}
	if support, err := configs.ConfigurationSupport(ctx, "core_other"); err != nil || !support.Structured {
		t.Fatalf("support of a 1.15 core = %+v, %v", support, err)
	}
	if support, err := configs.ConfigurationSupport(ctx, "core_older"); err != nil || support.Structured || support.Reason == "" {
		t.Fatalf("support of a 1.13.18 core = %+v, %v", support, err)
	}

	invalid := store.CanonicalRevision{Document: []byte(`{"log":{"level":"verbose"}}`)}
	if err := configs.ValidateForCore(ctx, "node", invalid, "core_native"); !errors.Is(err, ErrConfigurationSchemaValidation) {
		t.Fatalf("an invalid revision for a 1.14 core = %v", err)
	}
	for _, id := range []string{"core_other", "core_older"} {
		if err := configs.ValidateForCore(ctx, "node", invalid, id); err != nil {
			t.Fatalf("a revision for %s without a schema = %v, want it accepted", id, err)
		}
	}
}
