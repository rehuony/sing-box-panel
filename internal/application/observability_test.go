// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
