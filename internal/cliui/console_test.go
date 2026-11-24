// SPDX-License-Identifier: GPL-3.0-or-later
package cliui

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestForegroundOutputUsesCompactReadyBanner(t *testing.T) {
	var buffer bytes.Buffer
	output := ConsoleFrom(WithConsole(context.Background(), &buffer, false))
	output.Ready("sing-box-panel is running", []Field{
		{Label: "Panel URL", Value: "http://127.0.0.1:32123/panel/"},
		{Label: "Settings", Value: "/config/setting.json"},
		{Label: "Data directory", Value: "/data/panel"},
	}, "Press Ctrl+C to stop. View logs in the panel.", nil)
	want := "\nsing-box-panel is running\n\n" +
		"  Panel URL       http://127.0.0.1:32123/panel/\n" +
		"  Settings        /config/setting.json\n" +
		"  Data directory  /data/panel\n\n" +
		"  Press Ctrl+C to stop. View logs in the panel.\n\n"
	if buffer.String() != want {
		t.Fatalf("ready banner mismatch:\nwant %q\n got %q", want, buffer.String())
	}
	if strings.Contains(buffer.String(), "\x1b") {
		t.Fatal("redirected output has ANSI escapes")
	}
}

func TestStructuredOutputStaysMachineReadable(t *testing.T) {
	var buffer bytes.Buffer
	output := ConsoleFrom(WithConsole(context.Background(), &buffer, true))
	output.Ready("sing-box-panel is running", nil, "", map[string]string{"event": "server_ready", "panel_logs": "/data/panel.db"})
	output.Event(time.Now(), "warn", "example", "Example warning")
	decoder := json.NewDecoder(&buffer)
	var ready, event map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&event); err != nil {
		t.Fatal(err)
	}
	if ready["event"] != "server_ready" || ready["panel_logs"] != "/data/panel.db" || event["level"] != "warn" {
		t.Fatalf("ready=%v event=%v", ready, event)
	}
}
