// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// consoleKey carries a command's Console to the server it runs.
type consoleKey struct{}

// Console formats a foreground server's output, its log lines and its ready
// banner, without changing what the server stores.
type Console struct {
	mu         sync.Mutex
	writer     io.Writer
	structured bool
	style      Style
}

// WithConsole gives ctx a Console that writes to writer, as JSON documents
// when structured.
func WithConsole(ctx context.Context, writer io.Writer, structured bool) context.Context {
	format := Text
	if structured {
		format = JSON
	}
	output := &Console{writer: writer, structured: structured, style: NewStyle(writer, format)}
	return context.WithValue(ctx, consoleKey{}, output)
}

// ConsoleFrom returns the Console of ctx, or one on standard error.
func ConsoleFrom(ctx context.Context) *Console {
	if output, ok := ctx.Value(consoleKey{}).(*Console); ok {
		return output
	}
	return &Console{writer: os.Stderr}
}

// Style is how the output colors text.
func (output *Console) Style() Style { return output.style }

// Event prints one log entry: its time, its level colored by severity and
// its message.
func (output *Console) Event(at time.Time, level, code, message string) {
	output.mu.Lock()
	defer output.mu.Unlock()
	if output.structured {
		_ = json.NewEncoder(output.writer).Encode(map[string]any{"time": at, "level": level, "code": code, "message": message})
		return
	}
	label := output.style.Level(level, fmt.Sprintf("%-5s", strings.ToUpper(level)))
	_, _ = fmt.Fprintf(output.writer, "%s  %s  %s\n", at.Local().Format("15:04:05"), label, message)
}

// Ready announces a running server: title, aligned fields and a note as
// text, or event as one structured document.
func (output *Console) Ready(title string, fields []Field, note string, event any) {
	output.mu.Lock()
	defer output.mu.Unlock()
	if output.structured {
		_ = json.NewEncoder(output.writer).Encode(event)
		return
	}
	_, _ = fmt.Fprintln(output.writer, Notice(Section(output.style.Success(title), Fields(fields...), note)))
}
