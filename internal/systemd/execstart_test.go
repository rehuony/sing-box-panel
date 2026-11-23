// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import "testing"

func TestParseUnitFileFlagHandlesQuotingAndRefusesAmbiguity(t *testing.T) {
	cases := []struct {
		name string
		unit string
		want string
		ok   bool
	}{
		{name: "plain", unit: "[Unit]\nDescription=x\n[Service]\nExecStart=/usr/local/bin/sing-box-panel server start --config /etc/sing-box-panel/setting.json\n[Install]\nWantedBy=default.target\n", want: "/etc/sing-box-panel/setting.json", ok: true},
		{name: "quoted escapes", unit: "[Service]\nExecStart=\"/opt/bin/panel\" server start --config \"/tmp/paths with space;$$value%%/set\\\"ting\\\\.json\"\n", want: `/tmp/paths with space;$value%/set"ting\.json`, ok: true},
		{name: "equals form and prefix", unit: "[Service]\nExecStart=-/opt/bin/panel server start --config=/etc/a.json\n", want: "/etc/a.json", ok: true},
		{name: "continuation and reset", unit: "[Service]\nExecStart=/x --config /old.json\nExecStart=\nExecStart=/y server \\\n  --config /new.json\n", want: "/new.json", ok: true},
		{name: "single quotes shorthand", unit: "[Service]\nExecStart='/opt/bin/panel' server -c '/etc/b.json'\n", want: "/etc/b.json", ok: true},
		{name: "comments inside continuation", unit: `[Service]
ExecStart=/x --config /real.json \
# --config /comment.json
; --config /another-comment.json
  server
`, want: "/real.json", ok: true},
		{name: "attached shorthand", unit: "[Service]\nExecStart=/x server -c/etc/c.json\n", want: "/etc/c.json", ok: true},
		{name: "repeated flag last value wins", unit: "[Service]\nExecStart=/x server start --config /first.json --config /second.json\n", want: "/second.json", ok: true},
		{name: "double dash ends flags", unit: "[Service]\nExecStart=/x server start --config /kept.json -- --config /ignored.json\n", want: "/kept.json", ok: true},
		{name: "repeated flag with unresolvable value", unit: "[Service]\nExecStart=/x server start --config %h/a.json --config /b.json\n"},
		{name: "two effective commands", unit: "[Service]\nType=oneshot\nExecStart=/x server start --config /a.json\nExecStart=/x server start --config /b.json\n"},
		{name: "exec start outside service section", unit: "[Unit]\nExecStart=/x server start --config /a.json\n"},
		{name: "exec start before any section", unit: "ExecStart=/x server start --config /a.json\n"},
		{name: "unresolved specifier", unit: "[Service]\nExecStart=%h/.local/bin/sing-box-panel server start --config %h/.config/sing-box-panel/setting.json\n"},
		{name: "unresolved variable", unit: "[Service]\nExecStart=/x server start --config ${CONFIG}\n"},
		{name: "relative path", unit: "[Service]\nExecStart=/x server start --config setting.json\n"},
		{name: "missing value", unit: "[Service]\nExecStart=/x server start --config\n"},
		{name: "unterminated quote", unit: "[Service]\nExecStart=/x server start --config \"/etc/a.json\n"},
		{name: "no exec start", unit: "[Service]\nType=simple\n"},
		{name: "commented", unit: "[Service]\n# ExecStart=/x server start --config /etc/a.json\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := parseUnitFileFlag([]byte(testCase.unit), "--config", "-c")
			if ok != testCase.ok || got != testCase.want {
				t.Fatalf("parse = %q, %t; want %q, %t", got, ok, testCase.want, testCase.ok)
			}
		})
	}
}
