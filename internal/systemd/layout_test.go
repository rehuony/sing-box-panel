// SPDX-License-Identifier: GPL-3.0-or-later

package systemd

import (
	"errors"
	"testing"
)

// TestRootUnitWithoutHomeManagesTheSystemService builds the lifecycle the
// way a root unit such as the update helper runs it, without a home: system
// scope works and user scope is refused.
func TestRootUnitWithoutHomeManagesTheSystemService(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	manager, err := New(Definition[struct{}]{Name: "example", UserUnit: []byte("[Service]\n"), SystemStatePath: "/var/lib/example"}, Options{GOOS: "linux", EUID: func() int { return 0 }})
	if err != nil {
		t.Fatalf("a process without a home cannot manage the system service: %v", err)
	}
	if scope, err := manager.resolveScope(ScopeSystem); err != nil || scope != ScopeSystem {
		t.Fatalf("system scope = %q, %v", scope, err)
	}
	if _, err := manager.resolveScope(ScopeUser); !errors.Is(err, ErrInvalid) {
		t.Fatalf("user scope without a home = %v, want ErrInvalid", err)
	}
}
