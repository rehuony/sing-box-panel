// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
)

// A node updates from the panel it is enrolled with, so both run one
// version, and from the repository's releases only without a panel.
func TestUpdateSourceFollowsTheEnrollment(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "setting.json")
	if _, enrolled, err := currentEnrollment(path); err != nil || enrolled {
		t.Fatalf("missing settings: enrolled=%v err=%v", enrolled, err)
	}
	if _, err := settings.Initialize(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	// Keep the data directory inside the test, not at the user's default.
	dataDir := filepath.Join(root, "data")
	if err := settings.SetField(t.Context(), path, "data_dir", dataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, enrolled, err := currentEnrollment(path); err != nil || enrolled {
		t.Fatalf("unenrolled node: enrolled=%v err=%v", enrolled, err)
	}
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	saved := settings.Enrollment{URL: "https://panel.example/node", PanelID: "panel", NodeID: "node", Key: key}
	if err := settings.SaveEnrollment(dataDir, saved); err != nil {
		t.Fatal(err)
	}
	if enrollment, enrolled, err := currentEnrollment(path); err != nil || !enrolled || enrollment.NodeID != "node" {
		t.Fatalf("enrolled node: enrollment=%+v enrolled=%v err=%v", enrollment, enrolled, err)
	}
}
