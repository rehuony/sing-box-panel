// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"os"
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

func TestLocalMetricsQuotaAlwaysReadsFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "setting.json")
	if err := os.WriteFile(path, []byte(`{"data_dir":".","traffic":{"quota_gib":7,"sample_retention_days":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	quota, err := app.trafficQuota(t.Context())
	if err != nil || quota == nil || *quota != 7 {
		t.Fatalf("bootstrap quota = %v, %v", quota, err)
	}
	if err := os.WriteFile(path, []byte(`{"data_dir":".","traffic":{"quota_gib":-1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.trafficQuota(t.Context()); err == nil {
		t.Fatal("invalid bootstrap quota accepted")
	}
	for _, quotaJSON := range []string{"3", "null"} {
		if err := os.WriteFile(path, []byte(`{"data_dir":".","traffic":{"quota_gib":`+quotaJSON+`}}`), 0600); err != nil {
			t.Fatal(err)
		}
		quota, err = app.trafficQuota(t.Context())
		if err != nil || quotaJSON == "3" && (quota == nil || *quota != 3) || quotaJSON == "null" && quota != nil {
			t.Fatalf("file quota = %v, %v", quota, err)
		}
	}
}
