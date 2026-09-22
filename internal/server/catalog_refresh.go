// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

const (
	catalogRefreshRecheck = time.Minute
	catalogRefreshRetry   = 5 * time.Minute
)

type catalogRefreshCommands interface {
	Catalog(context.Context) (application.CatalogSnapshot, error)
	EffectiveSettings(context.Context) (settings.Settings, error)
	RefreshCatalog(context.Context, application.CatalogRefreshOptions) (application.CatalogSnapshot, error)
}

func startCatalogRefresh(ctx context.Context, commands catalogRefreshCommands) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runCatalogRefresh(ctx, commands, time.Now)
	}()
	return done
}

func runCatalogRefresh(ctx context.Context, commands catalogRefreshCommands, now func() time.Time) {
	for {
		wait := catalogRefreshStep(ctx, commands, now())
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func catalogRefreshStep(ctx context.Context, commands catalogRefreshCommands, now time.Time) time.Duration {
	configuration, settingsErr := commands.EffectiveSettings(ctx)
	snapshot, catalogErr := commands.Catalog(ctx)
	if ctx.Err() != nil {
		return catalogRefreshRecheck
	}
	force := catalogErr != nil
	due := force || settingsErr != nil
	if !due {
		interval := time.Duration(configuration.GitHub.CatalogRefreshIntervalHours) * time.Hour
		due = !now.UTC().Before(snapshot.RefreshedAt.Add(interval))
	}
	if !due {
		return catalogRefreshRecheck
	}
	if _, err := commands.RefreshCatalog(ctx, application.CatalogRefreshOptions{Force: force}); err != nil {
		return catalogRefreshRetry
	}
	return catalogRefreshRecheck
}
