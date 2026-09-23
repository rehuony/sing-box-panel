// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func startCoreLogRetention(ctx context.Context, commands *application.Application) <-chan struct{} {
	changes, unsubscribe := commands.SettingsChanges()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer unsubscribe()
		for {
			now := time.Now().UTC()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			timer := time.NewTimer(time.Until(midnight))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-changes:
				timer.Stop()
			case <-timer.C:
			}
			if err := commands.PruneCoreLogs(); err != nil {
				recordOperationalLog(commands, application.LogRecordRequest{Source: store.LogSourcePanel, Level: store.LogLevelError, Code: "core.log.retention_failed", Message: "Core log retention failed"})
			}
		}
	}()
	return done
}
