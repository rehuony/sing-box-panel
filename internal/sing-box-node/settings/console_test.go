// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleListensOnlyOnLoopback(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:3001", "127.0.0.2:3001", "[::1]:3001"} {
		if err := ValidateConsoleListen(listen); err != nil {
			t.Errorf("%s: %v", listen, err)
		}
	}
	for _, listen := range []string{"0.0.0.0:3001", "[::]:3001", "192.0.2.1:3001", "localhost:3001", "127.0.0.1:0", "127.0.0.1", "[fe80::1%lo0]:3001"} {
		if err := ValidateConsoleListen(listen); err == nil {
			t.Errorf("%s was accepted", listen)
		}
	}
	// A hand-edited file cannot expose the console either.
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, ConsoleFile), []byte(`{"enabled":false,"listen":"0.0.0.0:3001","password_hash":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConsole(dataDir); err == nil {
		t.Fatal("a console on every address was loaded")
	}
	if console, err := LoadConsole(t.TempDir()); err != nil || console.Enabled || console.Listen != DefaultConsoleListen {
		t.Fatalf("default console = %+v, %v", console, err)
	}
}
