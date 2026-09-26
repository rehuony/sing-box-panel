// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

type fakeCatalogRefreshCommands struct {
	configuration settings.Settings
	snapshot      application.CatalogSnapshot
	catalogErr    error
	refreshErr    error
	refreshes     []application.CatalogRefreshOptions
}

type blockingCatalogRefreshCommands struct {
	fakeCatalogRefreshCommands
	started chan struct{}
}

func (fake *blockingCatalogRefreshCommands) RefreshCatalog(ctx context.Context, _ application.CatalogRefreshOptions) (application.CatalogSnapshot, error) {
	close(fake.started)
	<-ctx.Done()
	return application.CatalogSnapshot{}, ctx.Err()
}

func (fake *fakeCatalogRefreshCommands) Catalog(context.Context) (application.CatalogSnapshot, error) {
	return fake.snapshot, fake.catalogErr
}

func (fake *fakeCatalogRefreshCommands) EffectiveSettings(context.Context) (settings.Settings, error) {
	return fake.configuration, nil
}

func (fake *fakeCatalogRefreshCommands) RefreshCatalog(_ context.Context, options application.CatalogRefreshOptions) (application.CatalogSnapshot, error) {
	fake.refreshes = append(fake.refreshes, options)
	return application.CatalogSnapshot{}, fake.refreshErr
}

func TestCatalogRefreshStepUsesPersistentStateAndDynamicInterval(t *testing.T) {
	now := time.Date(2026, time.September, 22, 10, 0, 0, 0, time.UTC)
	fake := &fakeCatalogRefreshCommands{configuration: settings.Defaults()}
	fake.snapshot.RefreshedAt = now.Add(-time.Hour)
	if wait := catalogRefreshStep(t.Context(), fake, now); wait != catalogRefreshRecheck || len(fake.refreshes) != 0 {
		t.Fatalf("fresh catalog wait=%s refreshes=%v", wait, fake.refreshes)
	}

	fake.configuration.GitHub.CatalogRefreshIntervalHours = 1
	if wait := catalogRefreshStep(t.Context(), fake, now); wait != catalogRefreshRecheck || len(fake.refreshes) != 1 || fake.refreshes[0].Force {
		t.Fatalf("due catalog wait=%s refreshes=%v", wait, fake.refreshes)
	}
}

func TestCatalogRefreshStepInitializesMissingStateAndRetriesFailure(t *testing.T) {
	fake := &fakeCatalogRefreshCommands{
		configuration: settings.Defaults(),
		catalogErr:    errors.New("catalog missing"),
		refreshErr:    errors.New("github unavailable"),
	}
	if wait := catalogRefreshStep(t.Context(), fake, time.Now()); wait != catalogRefreshRetry {
		t.Fatalf("failure wait=%s, want %s", wait, catalogRefreshRetry)
	}
	if len(fake.refreshes) != 1 || !fake.refreshes[0].Force {
		t.Fatalf("missing catalog refreshes=%v", fake.refreshes)
	}
}

func TestCatalogRefreshWorkerDoesNotBlockServerStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		fake := &blockingCatalogRefreshCommands{
			fakeCatalogRefreshCommands: fakeCatalogRefreshCommands{
				configuration: settings.Defaults(),
				catalogErr:    errors.New("catalog missing"),
			},
			started: make(chan struct{}),
		}
		done := startCatalogRefresh(ctx, fake)
		synctest.Wait()
		select {
		case <-fake.started:
		default:
			t.Fatal("background catalog refresh did not start")
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("background catalog refresh did not stop after cancellation")
		}
	})
}
