// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/corelogs"
)

func TestCoreLogMaintenanceCreatesIdleDailyFilesAndRetriesFailure(t *testing.T) {
	dataDir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		logs, err := corelogs.New(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		commands := &application.Application{}
		commands.SetCoreLogs(logs)
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
