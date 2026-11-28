// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// TestPanelCoreLogPolicyAppliesAndOutlivesARestart: the policy the panel
// sends removes what it no longer keeps at once, and the node starts with it
// again rather than with its default.
func TestPanelCoreLogPolicyAppliesAndOutlivesARestart(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "logs", "core")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Within the default seven days, beyond the panel's two.
	old := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02") + "-000.log"
	if err := os.WriteFile(filepath.Join(dir, old), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if starting, err := startingCoreLogPolicy(ctx, database); err != nil || starting != DefaultCoreLogPolicy {
		t.Fatalf("policy before the panel sent one = %+v, %v; want the default", starting, err)
	}
	node := FromStore(database)
	node.dataDir = dataDir
	policy := corelogs.Policy{RetentionDays: 2, MaxFiles: 5, MaxFileBytes: 2 << 20}
	if err := node.SetCoreLogPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, old)); !os.IsNotExist(err) {
		t.Fatalf("a capture outside the panel's retention remains: %v", err)
	}
	if stored, found, err := database.CoreLogPolicy(ctx); err != nil || !found ||
		stored != (store.CoreLogPolicy{RetentionDays: 2, MaxFiles: 5, MaxFileBytes: 2 << 20}) {
		t.Fatalf("stored policy = %+v, %v, %v", stored, found, err)
	}

	if starting, err := startingCoreLogPolicy(ctx, database); err != nil || starting != policy {
		t.Fatalf("policy after a restart = %+v, %v; want the panel's", starting, err)
	}
	if err := node.SetCoreLogPolicy(ctx, corelogs.Policy{RetentionDays: 0, MaxFileBytes: 1 << 20}); err == nil {
		t.Fatal("an invalid policy was applied")
	}
	if stored, _, err := database.CoreLogPolicy(ctx); err != nil || stored.RetentionDays != 2 {
		t.Fatalf("an invalid policy replaced the stored one: %+v, %v", stored, err)
	}
}

func TestCoreLogMaintenanceCreatesIdleDailyFilesAndRetriesFailure(t *testing.T) {
	dataDir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		logs, err := corelogs.New(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		commands := FromStore(nil)
		commands.attachCoreLogs(logs)
		ctx, cancel := context.WithCancel(t.Context())
		done := startCoreLogRetention(ctx, commands)
		defer func() { cancel(); <-done }()
		synctest.Wait()
		assertDailyFile := func(wantCount int) {
			t.Helper()
			files, err := logs.List()
			name := time.Now().UTC().Format("2006-01-02") + "-000.log"
			if err != nil || len(files) != wantCount || files[0].Name != name || files[0].Size != 0 {
				t.Fatalf("daily file: %+v, %v; want %s (%d files)", files, err, name, wantCount)
			}
		}
		assertDailyFile(1)
		now := time.Now().UTC()
		midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		time.Sleep(midnight.Sub(now))
		synctest.Wait()
		assertDailyFile(2)

		// A temporary filesystem failure at midnight must not delay recovery
		// until the following day, and no child output is needed to retry.
		dir := filepath.Join(dataDir, "logs", "core")
		if err := os.Rename(dir, dir+".offline"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(24 * time.Hour)
		synctest.Wait()
		if err := os.Rename(dir+".offline", dir); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		assertDailyFile(3)
	})
}
