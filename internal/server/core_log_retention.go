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
			if ctx.Err() != nil {
				return
			}
			now := time.Now().UTC()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			// Run at startup as well as midnight: daily captures must exist even
			// when the core is stopped or has emitted no new output.
			err := commands.MaintainCoreLogs()
			if err != nil {
				recordOperationalLog(commands, application.LogRecordRequest{Source: store.LogSourcePanel, Level: store.LogLevelError, Code: "core.log.maintenance_failed", Message: "Core log daily rotation or retention failed"})
			}
			// If maintenance crossed midnight, run again immediately rather
			// than postponing the new day's file until the following midnight.
			wait := time.Until(midnight)
			if err != nil {
				wait = min(wait, time.Minute)
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-changes:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()
	return done
}
