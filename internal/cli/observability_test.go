// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestMetricsCommandsReportCurrentAndHistoricalEvidence(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	period, err := database.UpsertTrafficPeriod(context.Background(), store.TrafficPeriod{
		ID: "period-cli", PeriodStart: now.Add(-time.Hour), PeriodEnd: now.Add(time.Hour),
		InboundBytes: 123, OutboundBytes: 456,
		Counters: json.RawMessage(`{"inbound":{"mixed":123}}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	metricsOutput := runApplicationCommand(t, settingsPath, "", "--output", "json", "metrics", "show")
	var metrics application.MetricsSnapshot
	if err := json.Unmarshal(metricsOutput, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.Available || metrics.ReasonCode != "not_applied" || metrics.CurrentTrafficData != nil {
		t.Fatalf("metrics fabricated unavailable values: %+v", metrics)
	}

	listOutput := runApplicationCommand(t, settingsPath, "", "--output", "json", "metrics", "history", "--limit", "1")
	var page struct {
		Items []store.TrafficPeriod `json:"items"`
	}
	if err := json.Unmarshal(listOutput, &page); err != nil || len(page.Items) != 1 || page.Items[0].ID != period.ID {
		t.Fatalf("metrics history=%+v err=%v output=%s", page, err, listOutput)
	}
	showOutput := runApplicationCommand(t, settingsPath, "", "--output", "json", "metrics", "period", period.ID)
	var shown store.TrafficPeriod
	if err := json.Unmarshal(showOutput, &shown); err != nil || shown.InboundBytes != 123 || shown.OutboundBytes != 456 {
		t.Fatalf("metrics period=%+v err=%v output=%s", shown, err, showOutput)
	}
}

func TestMetricsWatchRequiresStreamingOutput(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	var stdout bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &bytes.Buffer{},
		OpenApplication: application.Open,
	})
	command.SetArgs([]string{"--config", settingsPath, "--output", "json", "metrics", "watch"})
	err := command.ExecuteContext(context.Background())
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "jsonl") {
		t.Fatalf("metrics watch error=%v exit=%d", err, ExitCode(err))
	}
}

func TestMetricsTextShowsCurrentPeriodWithoutInventingTraffic(t *testing.T) {
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	snapshot := application.MetricsSnapshot{
		Available: true, TrafficAvailable: true, CollectedAt: start,
		CurrentTrafficData: &store.TrafficPeriod{
			ID: "internal-period-id", PeriodStart: start, PeriodEnd: end,
			InboundBytes: 123, OutboundBytes: 456,
		},
	}
	text := metricsText(snapshot)
	for _, want := range []string{"2026-09-01T00:00:00Z..2026-10-01T00:00:00Z", "in=123", "out=456"} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics %q lacks %q", text, want)
		}
	}
	if strings.Contains(text, snapshot.CurrentTrafficData.ID) {
		t.Fatalf("metrics shows internal identity instead of the period: %q", text)
	}
	snapshot.TrafficAvailable = false
	text = metricsText(snapshot)
	if !strings.Contains(text, "traffic=unavailable") || strings.Contains(text, "in=123") || strings.Contains(text, "out=456") {
		t.Fatalf("metrics fabricates traffic without counter evidence: %q", text)
	}
}
