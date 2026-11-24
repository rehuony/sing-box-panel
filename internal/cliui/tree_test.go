// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestTree(t *testing.T) {
	tests := []struct {
		name    string
		entries []TreeEntry
		want    string
	}{
		{
			name: "shared parents and compressed paths",
			entries: []TreeEntry{
				{Path: "/srv/panel/runtime/current.json", Label: "file"},
				{Path: "/srv/panel/imports/z.zip", Label: "file"},
				{Path: "/srv/panel/imports/nested/a.zip", Label: "file"},
				{Path: "/srv/panel/imports", Label: "dir", Directory: true},
				{Path: "/srv/panel/panel.db", Label: "file"},
			},
			want: "/\n" +
				"└── srv/panel/\n" +
				"    ├── imports/ [dir]\n" +
				"    │   ├── nested/a.zip [file]\n" +
				"    │   └── z.zip [file]\n" +
				"    ├── panel.db [file]\n" +
				"    └── runtime/current.json [file]",
		},
		{
			name: "separate groups under one filesystem root",
			entries: []TreeEntry{
				{Path: "/var/lib/panel/panel.db", Label: "file"},
				{Path: "/opt/panel/bin/panel", Label: "file"},
				{Path: "/etc/panel/setting.json", Label: "file"},
			},
			want: "/\n" +
				"├── etc/panel/setting.json [file]\n" +
				"├── opt/panel/bin/panel [file]\n" +
				"└── var/lib/panel/panel.db [file]",
		},
		{
			name: "explicit root and overlapping reports",
			entries: []TreeEntry{
				{Path: "/", Label: "dir"},
				{Path: "/a", Label: "service: unmanaged"},
				{Path: "/a", Label: "file"},
			},
			want: "/ [dir]\n└── a [file, service: unmanaged]",
		},
		{
			name: "sizes of files and the totals of reported directories",
			entries: []TreeEntry{
				{Path: "/etc/node/setting.json", Label: "settings", Size: 396, Sized: true},
				{Path: "/var/lib/node", Label: "data", Directory: true},
				{Path: "/var/lib/node/node.db", Size: 2048, Sized: true},
				{Path: "/var/lib/node/artifacts", Directory: true, Size: 3 << 20, Sized: true},
				{Path: "/var/lib/node/control.sock", Label: "socket"},
			},
			want: "/\n" +
				"├── etc/node/setting.json [settings, 396 B]\n" +
				"└── var/lib/node/ [data, 3.0 MiB]\n" +
				"    ├── artifacts/ [3.0 MiB]\n" +
				"    ├── control.sock [socket]\n" +
				"    └── node.db [2.0 KiB]",
		},
		{
			name:    "relative fixture paths",
			entries: []TreeEntry{{Path: "fixture.service", Label: "service: managed"}},
			want:    "./\n└── fixture.service [service: managed]",
		},
		{name: "empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (Style{}).Tree(test.entries); got != test.want {
				t.Fatalf("tree:\n%s\nwant:\n%s", got, test.want)
			}
			reversed := slices.Clone(test.entries)
			slices.Reverse(reversed)
			if got := (Style{}).Tree(reversed); got != test.want {
				t.Fatalf("tree depends on input order:\n%s", got)
			}
		})
	}
	colored := (Style{Color: true}).Tree(tests[0].entries)
	if got := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(colored, ""); got != tests[0].want {
		t.Fatalf("styling changed the tree:\n%s", got)
	}
	for _, want := range []string{"\x1b[34mimports/", "\x1b[2m    ├── "} {
		if !strings.Contains(colored, want) {
			t.Fatalf("missing semantic color %q in %q", want, colored)
		}
	}
}

func TestPathHomeAbbreviation(t *testing.T) {
	for _, test := range []struct{ home, path, want string }{
		{"/home/alice", "/home/alice", "~"},
		{"/home/alice/", "/home/alice/data", "~/data"},
		{"/home/alice", "/home/alice-other/data", "/home/alice-other/data"},
		{"/home/alice", "/home/alice2/data", "/home/alice2/data"},
		{"/home/alice", "/home", "/home"},
		{"/", "/etc/panel", "/etc/panel"},
		{"", "/etc/panel", "/etc/panel"},
		{"relative", "relative/panel", "relative/panel"},
	} {
		if got := (Style{Home: test.home}).Path(test.path); got != test.want {
			t.Fatalf("home=%q path=%q got=%q want=%q", test.home, test.path, got, test.want)
		}
	}
	entries := []TreeEntry{
		{Path: "/home/alice/config/setting.json", Label: "remove"},
		{Path: "/home/alice/data/panel.db", Label: "removed"},
	}
	want := "/\n└── home/alice/\n    ├── config/setting.json [remove]\n    └── data/panel.db [removed]"
	if got := (Style{Home: "/home/alice"}).Tree(entries); got != want {
		t.Fatalf("home paths were not grouped under the filesystem root:\n%s", got)
	}
}

type fdWriter struct {
	bytes.Buffer
	fdCalls int
}

func (writer *fdWriter) Fd() uintptr {
	writer.fdCalls++
	return ^uintptr(0) // Invalid descriptor: capability checks must fail closed.
}

func TestNewStyleDisablesColor(t *testing.T) {
	for _, test := range []struct {
		name, term, noColor string
		format              Format
		checkTerminal       bool
	}{
		{"terminal check", "xterm-256color", "", Text, true},
		{"no color", "xterm-256color", "1", Text, true},
		{"no color any value", "xterm", "0", Text, true},
		{"dumb", "dumb", "", Text, false},
		{"unknown terminal", "", "", Text, true},
		{"json", "xterm", "", JSON, false},
		{"jsonl", "xterm", "", JSONL, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TERM", test.term)
			t.Setenv("NO_COLOR", test.noColor)
			writer := &fdWriter{}
			style := NewStyle(writer, test.format)
			if style.Color || style.Home != "" || (writer.fdCalls > 0) != test.checkTerminal {
				t.Fatalf("style=%+v terminal checks=%d", style, writer.fdCalls)
			}
			if got := style.Label("removed"); got != "[removed]" {
				t.Fatalf("disabled color produced ANSI: %q", got)
			}
		})
	}
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	if style := NewStyle(&bytes.Buffer{}, Text); style.Color {
		t.Fatal("buffer output enabled color")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if style := NewStyle(writer, Text); style.Color || style.Home != "" {
		t.Fatal("pipe output enabled color or home abbreviation")
	}
}

func TestFieldsAlignValuesAndMarkUnavailable(t *testing.T) {
	got := Section("Title", Fields(Field{"Console", "http://127.0.0.1:8000/"}, Field{"Data directory", ""}), "Note")
	want := "Title\n\n  Console         http://127.0.0.1:8000/\n  Data directory  —\n\n  Note"
	if got != want {
		t.Fatalf("section:\n%q\nwant:\n%q", got, want)
	}
}
