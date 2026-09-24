// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"strings"
	"testing"
)

func TestSystemUnitExposesOnlyReadOnlyHostCounters(t *testing.T) {
	unit, err := renderUnit(ScopeSystem, "/usr/local/bin/sing-box-panel", "/etc/sing-box-panel/setting.json", "/var/lib/sing-box-panel")
	if err != nil {
		t.Fatal(err)
	}
	for _, directive := range []string{
		"ProtectProc=invisible",
		"ProcSubset=pid",
		"Environment=SING_BOX_PANEL_HOST_PROC=/run/sing-box-panel/host-proc",
		"BindReadOnlyPaths=/proc/stat:/run/sing-box-panel/host-proc/stat /proc/meminfo:/run/sing-box-panel/host-proc/meminfo /proc/loadavg:/run/sing-box-panel/host-proc/loadavg",
	} {
		if !strings.Contains(string(unit), directive+"\n") {
			t.Fatalf("unit is missing %q", directive)
		}
	}
}
