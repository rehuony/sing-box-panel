// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
)

// Style decides how text output looks: whether it is colored, and the home
// directory that paths are shown relative to. The zero value is plain.
type Style struct {
	Color bool
	Home  string
}

// NewStyle colors text output to a terminal unless NO_COLOR is set or TERM
// is empty or dumb, and shows paths below the home directory as ~/... there;
// redirected and structured output keep plain absolute paths.
func NewStyle(writer io.Writer, format Format) Style {
	var style Style
	if format == Text && Interactive(writer) {
		style.Color = colorAllowed()
		style.Home, _ = os.UserHomeDir()
	}
	return style
}

func colorAllowed() bool {
	term := os.Getenv("TERM")
	return os.Getenv("NO_COLOR") == "" && term != "" && term != "dumb"
}

// Interactive reports whether writer is a terminal that can redraw a line.
func Interactive(writer io.Writer) bool {
	terminal, ok := writer.(interface{ Fd() uintptr })
	return ok && os.Getenv("TERM") != "dumb" && isatty.IsTerminal(terminal.Fd())
}

// The semantic colors of every command's text output.
const (
	codeBold      = "1"
	codeSuccess   = "1;32"
	codeFailure   = "1;31"
	codeWarning   = "1;33"
	codeHint      = "1;36"
	codeAccent    = "36"
	codeMuted     = "2"
	codeDirectory = "34"
	codeGood      = "32"
	codeCaution   = "33"
	codeBad       = "31"
)

func (style Style) paint(code, text string) string {
	if !style.Color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Title marks a neutral heading; Success, Failure and Warning mark the
// heading of a result that succeeded, failed or needs attention.
func (style Style) Title(text string) string   { return style.paint(codeBold, text) }
func (style Style) Success(text string) string { return style.paint(codeSuccess, text) }
func (style Style) Failure(text string) string { return style.paint(codeFailure, text) }
func (style Style) Warning(text string) string { return style.paint(codeWarning, text) }

// Accent marks a value to act on, such as a URL; Muted marks secondary text.
func (style Style) Accent(text string) string { return style.paint(codeAccent, text) }
func (style Style) Muted(text string) string  { return style.paint(codeMuted, text) }

// Level colors a log level: errors red, warnings yellow, the rest cyan.
func (style Style) Level(level, text string) string {
	switch strings.ToLower(level) {
	case "error", "fatal":
		return style.paint(codeBad, text)
	case "warn", "warning":
		return style.paint(codeCaution, text)
	}
	return style.paint(codeAccent, text)
}

// Hint prefixes text with a highlighted label such as "Next:".
func (style Style) Hint(label, text string) string {
	return style.paint(codeHint, label) + " " + text
}

// Next names the command that continues the work.
func (style Style) Next(command string) string { return style.Hint("Next:", command) }

// Label renders comma-separated labels in brackets, each colored by meaning.
func (style Style) Label(label string) string {
	parts := strings.Split(label, ", ")
	for i, part := range parts {
		code := codeMuted
		switch part {
		case "link", "outside scope", "retained":
			code = codeAccent
		case "removed":
			code = codeGood
		case "remaining", "unknown":
			code = codeCaution
		}
		parts[i] = style.paint(code, part)
	}
	return style.paint(codeMuted, "[") + strings.Join(parts, style.paint(codeMuted, ", ")) + style.paint(codeMuted, "]")
}

// Path shows path relative to the home directory as ~/...
func (style Style) Path(path string) string {
	home := filepath.Clean(style.Home)
	if style.Home != "" && filepath.IsAbs(home) && home != string(filepath.Separator) &&
		(path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
