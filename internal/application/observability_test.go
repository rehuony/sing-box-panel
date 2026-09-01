// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestMetricsReportsMissingEvidenceInsteadOfZeroCounters(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.August, 26, 22, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }

	metrics, err := app.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Available || metrics.ReasonCode != "not_applied" || metrics.CurrentTrafficData != nil {
		t.Fatalf("metrics = %+v", metrics)
	}
}

func TestTrafficSampleRetentionUsesConfiguredDays(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	configuration := settings.Defaults()
	configuration.Traffic.SampleRetentionDays = 30
	app := FromStoreWithSettings(database, configuration)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }
	result, err := app.EnforceTrafficSampleRetention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantCutoff := now.Add(-30 * 24 * time.Hour)
	if result.Deleted != 0 || !result.Cutoff.Equal(wantCutoff) {
		t.Fatalf("retention = %+v, want cutoff %s", result, wantCutoff)
	}
	app.settings.Traffic.SampleRetentionDays = 0
	if _, err := app.EnforceTrafficSampleRetention(ctx); err == nil {
		t.Fatal("invalid retention setting succeeded")
	}
}
