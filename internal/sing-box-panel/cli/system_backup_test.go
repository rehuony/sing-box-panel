// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

// TestSystemBackupAndRestoreAStoppedInstance backs up a stopped instance
// with a passphrase, previews the backup without changing anything, and
// restores its nodes with --yes, backing up the state it replaces first.
func TestSystemBackupAndRestoreAStoppedInstance(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	database, _ := openPanelFixture(t, settingsPath)
	testutil.CreateNode(t, database, "edge")
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	passphrase := filepath.Join(t.TempDir(), "passphrase")
	if err := os.WriteFile(passphrase, []byte("correct horse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(t.TempDir(), "panel.tar.gz")
	if _, _, err := execute(t, "--config", settingsPath, "system", "backup", "--categories", "sessions"); err == nil {
		t.Fatal("backup of an unknown category succeeded")
	}
	stdout, stderr, err := execute(t, "--config", settingsPath, "system", "backup", "--passphrase-file", passphrase, "--output", copied)
	if err != nil || !strings.Contains(stdout, copied) {
		t.Fatalf("backup: %q %q %v", stdout, stderr, err)
	}

	// The node is removed after the backup; the restore brings it back.
	database, _ = openPanelFixture(t, settingsPath)
	nodes, err := database.Nodes(t.Context())
	if err != nil || len(nodes) != 1 {
		t.Fatal(nodes, err)
	}
	if err := database.DeleteNode(t.Context(), nodes[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execute(t, "--config", settingsPath, "system", "restore", copied); err == nil {
		t.Fatal("restore without the passphrase succeeded")
	}
	stdout, _, err = execute(t, "--config", settingsPath, "system", "restore", copied, "--passphrase-file", passphrase)
	if err != nil || !strings.Contains(stdout, "1 nodes") || !strings.Contains(stdout, "--yes") {
		t.Fatalf("preview: %q %v", stdout, err)
	}
	stdout, _, err = execute(t, "--config", settingsPath, "system", "restore", copied, "--passphrase-file", passphrase, "--categories", "nodes", "--yes")
	if err != nil || !strings.Contains(stdout, "Restored.") {
		t.Fatalf("restore: %q %v", stdout, err)
	}
	database, _ = openPanelFixture(t, settingsPath)
	nodes, err = database.Nodes(t.Context())
	_ = database.Close()
	if err != nil || len(nodes) != 1 || nodes[0].Name != "edge" {
		t.Fatalf("restored nodes = %+v, %v", nodes, err)
	}
}
