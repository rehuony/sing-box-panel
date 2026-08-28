// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func TestSubscriptionSourceRefreshTaskPublishesOnlySuccessfulVersion(t *testing.T) {
	ctx := context.Background()
	response := `[{"type":"socks","tag":"remote","server":"remote.example","server_port":1080}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)
	app.settings = settings.Defaults(filepath.Join(t.TempDir(), "setting.json"))
	app.settings.Subscription.PrivateSourceCIDRs = []string{"127.0.0.1/32"}
	source, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "remote", SourceKind: store.SubscriptionSourceRemote, Enabled: true,
		Config: json.RawMessage(`{"url":"` + server.URL + `","format":"sing-box-json"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := app.QueueSubscriptionSourceRefresh(ctx, source.ID)
	if err != nil || queued.Kind != "subscription-source-refresh" || queued.Status != store.TaskStatusQueued {
		t.Fatalf("queued=%+v err=%v", queued, err)
	}
	result, err := app.ExecuteSubscriptionSourceRefresh(ctx, queued.Payload, nil)
	if err != nil || result.SourceID != source.ID || result.VersionID == "" || result.NodeCount != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	current, err := app.SubscriptionSource(ctx, source.ID)
	if err != nil || current.CurrentVersionID != result.VersionID {
		t.Fatalf("current=%+v err=%v", current, err)
	}

	// A malformed refresh candidate cannot replace the current successful
	// version, even though the HTTP request itself succeeded.
	response = `{"outbounds":[{"type":"trojan","tag":"broken","server":"broken.example","server_port":443}]}`
	retry, err := app.QueueSubscriptionSourceRefresh(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecuteSubscriptionSourceRefresh(ctx, retry.Payload, nil); err == nil {
		t.Fatal("malformed source candidate was accepted")
	}
	unchanged, err := app.SubscriptionSource(ctx, source.ID)
	if err != nil || unchanged.CurrentVersionID != current.CurrentVersionID {
		t.Fatalf("failed refresh replaced current version: %+v err=%v", unchanged, err)
	}
}

func TestRemoteSubscriptionSourceConfigRequiresExplicitScheduleMinimum(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newSubscriptionTestApplication(database)
	if _, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "too frequent", SourceKind: store.SubscriptionSourceRemote,
		Config:  json.RawMessage(`{"url":"https://example.test/sub","format":"` + string(subscription.SourceFormatAuto) + `","refresh_interval_minutes":14}`),
		Enabled: true,
	}); err == nil {
		t.Fatal("sub-15-minute source schedule was accepted")
	}
	scheduled, err := app.CreateSubscriptionSource(ctx, CreateSubscriptionSourceRequest{
		Name: "scheduled", SourceKind: store.SubscriptionSourceRemote,
		Config:  json.RawMessage(`{"url":"https://example.test/sub","refresh_interval_minutes":15}`),
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := database.ListTasks(ctx, store.TaskListFilter{Kind: "subscription-source-refresh"})
	if err != nil || len(tasks.Items) != 1 || tasks.Items[0].NotBefore == nil ||
		!tasks.Items[0].NotBefore.Equal(app.now().UTC().Add(15*time.Minute)) ||
		!strings.Contains(string(tasks.Items[0].Payload), scheduled.ID) {
		t.Fatalf("scheduled refresh tasks=%+v err=%v", tasks, err)
	}
}
