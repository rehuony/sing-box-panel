// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestLiveTrafficSamplingPreservesPersistenceAndRegressionEvidence(t *testing.T) {
	var counter atomic.Int64
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			w.WriteHeader(503)
			return
		}
		if r.Header.Get("Authorization") != "Bearer collector-test" {
			t.Error("missing collector authentication")
		}
		fmt.Fprintf(w, `{"uploadTotal":%d,"downloadTotal":%d,"memory":100,"connections":[{}]}`, counter.Load(), counter.Load()*2)
	}))
	defer server.Close()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := newSubscriptionTestApplication(db)
	defer app.CloseTrafficCollector()
	at := time.Date(2026, 8, 31, 23, 59, 50, 0, time.UTC)
	app.now = func() time.Time { return at }
	core := store.CoreArtifact{ID: "collector-core", ExactVersion: "1.13.19", OperatingSystem: "linux", Architecture: "arm64", Variant: "musl",
		SourceKind: store.CoreArtifactSourceUserVerified, UserSource: "test", ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/tmp/sing-box", ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":["with_clash_api"]}`), CreatedAt: at}
	if _, err = db.UpsertCoreArtifact(t.Context(), core); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"experimental":{"clash_api":{"external_controller":%q,"secret":"collector-test"}}}`, strings.TrimPrefix(server.URL, "http://"))
	saved, err := app.SaveConfigurationFile(t.Context(), ConfigurationFileWrite{Content: config})
	if err != nil {
		t.Fatal(err)
	}
	startup, err := db.CreateStartupArtifact(t.Context(), store.StartupArtifact{ID: "collector-startup", CanonicalRevisionID: saved.CanonicalRevisionID, CoreArtifactID: core.ID, ExactCoreVersion: core.ExactVersion, ConfigBytes: []byte(config), CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CompleteStartupArtifactCheck(t.Context(), startup.ID, true, at); err != nil {
		t.Fatal(err)
	}
	bundle, err := db.SaveActivationBundle(t.Context(), store.ActivationBundle{ID: "collector-bundle", StartupArtifactID: startup.ID, MonitoringTier: store.MonitoringLimited, CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	observation := store.RuntimeObservation{PID: 42, ProcessStartToken: "collector-process", ActivationBundleID: bundle.ID, CoreArtifactID: core.ID, ExactCoreVersion: core.ExactVersion, ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256, StartedAt: at, ObservedAt: at}
	if _, err = db.RecordRuntimeObservation(t.Context(), observation); err != nil {
		t.Fatal(err)
	}
	collect := func(value int64) store.TrafficSampleResult {
		t.Helper()
		counter.Store(value)
		result, err := app.SampleLimitedTraffic(t.Context(), observation, false)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := collect(100)
	pooled := app.collector.client
	if !first.Sample.Accepted {
		t.Fatal(first)
	}
	for i := 1; i <= 4; i++ {
		at = at.Add(2 * time.Second)
		if result := collect(int64(100 + i*10)); result.Sample.ID != 0 {
			t.Fatal("intermediate live sample entered accounting")
		}
	}
	if app.collector.live.Load().UploadTotal != 140 || app.collector.client != pooled {
		t.Fatal("live sampling or process-client reuse failed")
	}
	latest, err := db.LatestAcceptedTrafficSample(t.Context())
	if err != nil || latest.ID != first.Sample.ID {
		t.Fatal("live samples changed persistence", err)
	}
	at = at.Add(2 * time.Second) // UTC month boundary
	persisted := collect(150)
	if persisted.Sample.UploadDelta == nil || *persisted.Sample.UploadDelta != 50 || persisted.Sample.Coverage != store.CoveragePartial {
		t.Fatalf("cross-month evidence=%+v", persisted.Sample)
	}
	at = at.Add(2 * time.Second)
	collect(170)
	at = at.Add(2 * time.Second)
	rejected := collect(160)
	if rejected.Sample.Accepted || rejected.Sample.DiagnosticCode != "counter_decreased" {
		t.Fatalf("regression hidden by decimation: %+v", rejected.Sample)
	}
	if app.collector.live.Load().Accepted {
		t.Fatal("regressed live evidence accepted")
	}
	at = at.Add(2 * time.Second)
	counter.Store(180)
	unavailable.Store(true)
	if _, err = app.SampleLimitedTraffic(t.Context(), observation, false); err == nil || app.collector.live.Load() != nil {
		t.Fatal("failed collection retained live evidence")
	}
	unavailable.Store(false)
	at = at.Add(10 * time.Second)
	recovered := collect(190)
	if recovered.Sample.UploadDelta == nil || *recovered.Sample.UploadDelta != 20 {
		t.Fatalf("regression corrupted checkpoint: %+v", recovered.Sample)
	}
	duplicate := collect(190)
	if duplicate.Sample.Accepted || duplicate.Sample.DiagnosticCode != "sample_out_of_order" {
		t.Fatalf("duplicate time=%+v", duplicate.Sample)
	}
	observation.ProcessStartToken = "restarted-process"
	if _, err = db.RecordRuntimeObservation(t.Context(), observation); err != nil {
		t.Fatal(err)
	}
	at = at.Add(2 * time.Second)
	restarted := collect(5)
	if !restarted.Sample.Accepted || restarted.Sample.UploadDelta != nil || app.collector.client == pooled {
		t.Fatalf("incarnation transition=%+v", restarted.Sample)
	}
}
