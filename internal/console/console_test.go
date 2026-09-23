// SPDX-License-Identifier: GPL-3.0-or-later
package console

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
	output := FromContext(WithOutput(context.Background(), &buffer, false))
	output.Ready("http://127.0.0.1:32123/panel/", "/config/setting.json", "/data/panel")
	want := "\nsing-box-panel is running\n\n" +
		"  Panel URL  http://127.0.0.1:32123/panel/\n" +
		"  Settings   /config/setting.json\n" +
		"  Data Dir   /data/panel\n\n" +
		"  Press Ctrl+C to stop. View stored logs: sing-box-panel log list\n\n"
	if buffer.String() != want {
		t.Fatalf("ready banner mismatch:\nwant %q\n got %q", want, buffer.String())
	}
	if strings.Contains(buffer.String(), "\x1b") {
		t.Fatal("redirected output has ANSI escapes")
	}
}

func TestStructuredOutputStaysMachineReadable(t *testing.T) {
	var buffer bytes.Buffer
	output := FromContext(WithOutput(context.Background(), &buffer, true))
	output.Ready("http://127.0.0.1:3000/", "/setting.json", "/data")
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
