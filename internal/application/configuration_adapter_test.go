// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestPreviewConfigurationUsesExactCompiledProfile(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := FromStore(database)
	now := time.Date(2026, 8, 28, 1, 2, 3, 0, time.UTC)
	application.now = func() time.Time { return now }

	features, err := json.Marshal(adapter.FeatureFingerprint{Status: "reported", Features: []string{
		"badlinkname", "tfogo_checklinkname0", "with_acme", "with_ccm", "with_clash_api",
		"with_dhcp", "with_gvisor", "with_ocm", "with_quic", "with_tailscale", "with_utls", "with_wireguard",
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.UpsertCoreArtifact(ctx, store.CoreArtifact{
		ID: "core_11319", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: features, VerificationState: store.CoreArtifactVerified, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	document := canonical.EmptyV2().CanonicalJSON()
	_, err = database.SaveCanonicalRevisionAndTask(ctx, "", store.NewCanonicalRevision{
		ID: "rev_1", SchemaVersion: canonical.SchemaVersionV2, Document: document, CommandID: "cmd_1", CreatedAt: now,
	}, store.NewTask{ID: "task_1", Lane: store.TaskLaneMaintenance, Kind: "canonical-saved", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	preview, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{CoreArtifactID: "core_11319"})
	if err != nil {
		t.Fatalf("PreviewConfiguration() error = %v", err)
	}
	if !preview.Support.Supported || preview.Support.AdapterID != "sing-box/v1_13_19/official-linux-plain" || preview.Support.Revision != "2" {
		t.Fatalf("support = %+v", preview.Support)
	}
	if string(preview.Config) != "{}" {
		t.Fatalf("config = %s", preview.Config)
	}
}

func TestConfigurationSupportFailsClosedForUnreviewedFingerprint(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := FromStore(database)
	now := time.Now().UTC()
	_, err = database.UpsertCoreArtifact(ctx, store.CoreArtifact{
		ID: "core_unknown", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"not_reported"}`), VerificationState: store.CoreArtifactVerified, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	support, err := application.ConfigurationSupport(ctx, "core_unknown")
	if err != nil {
		t.Fatal(err)
	}
	if support.Supported || !strings.Contains(support.Reason, adapter.ErrUnsupportedCoreProfile.Error()) {
		t.Fatalf("support = %+v", support)
	}
	_, err = database.SaveCanonicalRevisionAndTask(ctx, "", store.NewCanonicalRevision{
		ID: "rev_unknown", SchemaVersion: canonical.SchemaVersionV2,
		Document: canonical.EmptyV2().CanonicalJSON(), CommandID: "cmd_unknown", CreatedAt: now,
	}, store.NewTask{
		ID: "task_unknown", Lane: store.TaskLaneMaintenance, Kind: "canonical-saved", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.PreviewConfiguration(ctx, ConfigurationPreviewRequest{
		CoreArtifactID: "core_unknown",
	}); !errors.Is(err, adapter.ErrUnsupportedCoreProfile) {
		t.Fatalf("PreviewConfiguration error = %v, want ErrUnsupportedCoreProfile", err)
	}
}
