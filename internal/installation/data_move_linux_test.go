// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package installation

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestDataMoveAcrossFilesystems(t *testing.T) {
	path, source := fixture(t)
	target, err := os.MkdirTemp("/dev/shm", "sbp-data-move-")
	if err != nil {
		t.Skip("second filesystem unavailable", err)
	}
	t.Cleanup(func() { os.RemoveAll(target) })
	from, _ := os.Stat(source)
	to, _ := os.Stat(target)
	if from.Sys().(*syscall.Stat_t).Dev == to.Sys().(*syscall.Stat_t).Dev {
		t.Skip("fixture needs two filesystems")
	}
	putFile(t, filepath.Join(source, "logs/core/example.log"))
	selectMovedDataDir(t, path, target)
	if _, err := PrepareDataLocation(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "logs/core/example.log"))
	if err != nil || string(data) != "fixture" {
		t.Fatal("cross-filesystem move lost data", err)
	}
}

func TestDataMoveRefusesOrphanedLiveCore(t *testing.T) {
	path, source := fixture(t)
	db, err := store.Open(t.Context(), filepath.Join(source, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := application.NewRuntimeIdentityResolver(db).ProcessStartToken(t.Context(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		digest := strings.Repeat("a", 64)
		statements := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO canonical_revisions(id,sequence,schema_version,document_json,sha256,command_id,created_at) VALUES('revision',1,1,'{}',?,'command','2026-09-19T00:00:00Z')`, []any{digest}},
			{`INSERT INTO core_artifacts(id,exact_version,operating_system,architecture,source_kind,user_source,archive_sha256,binary_sha256,binary_path,reported_version,created_at) VALUES('core','1.14.0','linux','arm64','user_verified','fixture',?,?,?,'1.14.0','2026-09-19T00:00:00Z')`, []any{digest, digest, filepath.Join(source, "core")}},
			{`INSERT INTO startup_artifacts(id,canonical_revision_id,exact_core_version,core_artifact_id,config_bytes,config_sha256,created_at) VALUES('startup','revision','1.14.0','core',x'7b7d',?,'2026-09-19T00:00:00Z')`, []any{digest}},
			{`INSERT INTO activation_bundles(id,startup_artifact_id,monitoring_tier,sha256,created_at) VALUES('bundle','startup','process_only',?,'2026-09-19T00:00:00Z')`, []any{digest}},
			{`INSERT INTO runtime_observation(singleton,pid,process_start_token,core_artifact_id,activation_bundle_id,exact_core_version,archive_sha256,binary_sha256,started_at,observed_at) VALUES(1,?,?,'core','bundle','1.14.0',?,?,'2026-09-19T00:00:00Z','2026-09-19T00:00:00Z')`, []any{os.Getpid(), token, digest, digest}},
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(t.Context(), statement.sql, statement.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	selectMovedDataDir(t, path, filepath.Join(filepath.Dir(source), "target"))
	if _, err := PrepareDataLocation(t.Context(), path); err == nil || !strings.Contains(err.Error(), "stop the managed core") {
		t.Fatalf("live orphan accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "panel.db")); err != nil {
		t.Fatal("source changed", err)
	}
}
