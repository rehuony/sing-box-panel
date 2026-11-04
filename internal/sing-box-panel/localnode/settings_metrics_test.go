package localnode

import (
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
)

func TestQuotaVisibleWithoutAppliedCoreAndDynamicPoliciesDoNotRequireRestart(t *testing.T) {
	app := panelFileApp(t)
	view, err := app.panelSettings.PanelSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	quota := int64(123)
	view.Preferences.TrafficQuotaGiB = &quota
	view.Service.TrafficPeriodMonths = 3
	view.Service.SampleRetentionDays = 2
	changes, cancel := app.panelSettings.SettingsChanges()
	defer cancel()
	saved, err := app.panelSettings.SavePanelSettings(t.Context(), settings.PanelSettingsWrite{Revision: view.Revision, Preferences: view.Preferences, Service: &view.Service})
	if err != nil || saved.Settings.RestartRequired {
		t.Fatalf("dynamic settings: %+v %v", saved, err)
	}
	select {
	case <-changes:
	default:
		t.Fatal("settings consumers were not notified")
	}
	retention, err := app.telemetry.EnforceTrafficSampleRetention(t.Context())
	if err != nil || app.now().UTC().Sub(retention.Cutoff) < 48*time.Hour || app.now().UTC().Sub(retention.Cutoff) > 48*time.Hour+time.Second {
		t.Fatalf("retention did not hot reload: %+v %v", retention, err)
	}
	metrics, err := app.Metrics(t.Context())
	if err != nil || metrics.QuotaBytes == nil || *metrics.QuotaBytes != quota<<30 || metrics.CurrentTrafficData.InboundBytes != 0 || metrics.CurrentTrafficData.OutboundBytes != 0 {
		t.Fatalf("quota without traffic: %+v %v", metrics, err)
	}
}
