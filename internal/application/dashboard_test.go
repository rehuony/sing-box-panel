// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestDashboardSnapshotUsesStableRangesWithoutInventingEvidence(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	app.now = func() time.Time { return now }

	snapshot, err := app.DashboardSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.CollectedAt.Equal(now) || snapshot.History1H.BucketSeconds != 60 || snapshot.History24H.BucketSeconds != 300 {
		t.Fatalf("dashboard snapshot=%+v", snapshot)
	}
	if !snapshot.History1H.From.Equal(now.Add(-time.Hour)) || !snapshot.History24H.From.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("dashboard ranges 1h=%s 24h=%s", snapshot.History1H.From, snapshot.History24H.From)
	}
	for _, bucket := range append(snapshot.History1H.Buckets, snapshot.History24H.Buckets...) {
		if bucket.UploadBytes != nil || bucket.DownloadBytes != nil || bucket.ActiveConnectionsAverage != nil || bucket.Coverage != store.CoverageMissing {
			t.Fatalf("empty dashboard invented metric evidence: %+v", bucket)
		}
	}
	if len(snapshot.Runtime24H.Items) != 1 || len(snapshot.Activity.Items) != 1 {
		t.Fatalf("dashboard did not include initialized runtime evidence: %+v", snapshot)
	}
}
