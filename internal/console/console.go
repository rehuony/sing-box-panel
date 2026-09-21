// SPDX-License-Identifier: GPL-3.0-or-later

// Package console formats foreground server output without changing stored logs.
package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

type contextKey struct{}

type Output struct {
	mu         sync.Mutex
	writer     io.Writer
	structured bool
	color      bool
}

func WithOutput(ctx context.Context, writer io.Writer, structured bool) context.Context {
	output := &Output{writer: writer, structured: structured}
	if terminal, ok := writer.(interface{ Fd() uintptr }); ok {
		output.color = !structured && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "" && os.Getenv("TERM") != "dumb" && isatty.IsTerminal(terminal.Fd())
	}
	return context.WithValue(ctx, contextKey{}, output)
}

func FromContext(ctx context.Context) *Output {
	if output, ok := ctx.Value(contextKey{}).(*Output); ok {
		return output
	}
	return &Output{writer: os.Stderr}
}

func (output *Output) Event(at time.Time, level, code, message string) {
	output.mu.Lock()
	defer output.mu.Unlock()
	if output.structured {
		_ = json.NewEncoder(output.writer).Encode(map[string]any{"time": at, "level": level, "code": code, "message": message})
		return
	}
	label := strings.ToUpper(level)
	if output.color {
		color := "36"
		if level == "error" || level == "fatal" {
			color = "31"
		} else if level == "warn" {
			color = "33"
		}
		label = "\x1b[" + color + "m" + fmt.Sprintf("%-5s", label) + "\x1b[0m"
	}
	_, _ = fmt.Fprintf(output.writer, "%s  %-5s  %s\n", at.Local().Format("15:04:05"), label, message)
}

func (output *Output) Ready(url, settingsPath, dataDir string) {
	output.mu.Lock()
	defer output.mu.Unlock()
	if output.structured {
		_ = json.NewEncoder(output.writer).Encode(map[string]string{"event": "server_ready", "panel_url": url, "settings_path": settingsPath, "data_dir": dataDir, "panel_logs": dataDir + "/panel.db"})
		return
	}
	title := "sing-box-panel is running"
	if output.color {
		title = "\x1b[1;32m" + title + "\x1b[0m"
		url = "\x1b[36m" + url + "\x1b[0m"
	}
	_, _ = fmt.Fprintf(output.writer, "\n  %s\n\n  Panel URL    %s\n  Settings     %s\n  Data         %s\n  Panel logs   %s/panel.db (also shown below)\n\n  Press Ctrl+C to stop. View stored logs: sing-box-panel log list\n\n", title, url, settingsPath, dataDir, dataDir)
}
