// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestPublicHostChangeReportsPublicationChange(t *testing.T) {
	dataDir := t.TempDir()
	database, err := store.Open(t.Context(), filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults()
	value.DataDir = dataDir
	panelSettings := settings.NewService(database, "", value)
	records := telemetry.New(database, panelSettings, telemetry.Options{})
	cores := corecache.New(database, records, panelSettings, corecache.Options{DataDir: dataDir})
	changes := 0
	service := fleet.New(database, panelSettings, records, nodeconfig.New(database, cores, nil), cores, fleet.Options{
		PublicationChanged: func(context.Context) { changes++ },
	})
	id := testutil.CreateNode(t, database, "edge")
	save := func(host string, quota int64) {
		t.Helper()
		current, err := service.NodeSettings(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		current.PublicHost, current.Traffic.QuotaGiB = host, &quota
		if _, err := service.SaveNodeSettings(t.Context(), id, current); err != nil {
			t.Fatal(err)
		}
	}
	save("edge.example", 1)
	save("edge.example", 2)
	if changes != 1 {
		t.Fatalf("changes = %d, want only the public host change", changes)
	}
	save("", 2)
	if changes != 2 {
		t.Fatalf("changes = %d after clearing the public host", changes)
	}
}
