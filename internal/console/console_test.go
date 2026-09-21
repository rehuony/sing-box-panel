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

func TestForegroundOutputShowsAddressPathsAndLogs(t *testing.T) {
	var buffer bytes.Buffer
	output := FromContext(WithOutput(context.Background(), &buffer, false))
	output.Ready("http://127.0.0.1:32123/panel/", "/config/setting.json", "/data/panel")
	output.Event(time.Now(), "info", "panel.ready", "Panel server is ready")
	for _, expected := range []string{"http://127.0.0.1:32123/panel/", "/config/setting.json", "/data/panel/panel.db", "Ctrl+C", "INFO", "Panel server is ready"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Fatalf("missing %q in %s", expected, buffer.String())
		}
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
	if ready["event"] != "server_ready" || event["level"] != "warn" {
		t.Fatalf("ready=%v event=%v", ready, event)
	}
}
