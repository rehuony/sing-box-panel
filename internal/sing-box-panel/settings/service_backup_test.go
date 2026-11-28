// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

// TestRestoredDocumentKeepsThisHostsDataDirectoryAndListener writes a
// backup's settings with this installation's data directory, and its
// listener unless the restore takes the backup's.
func TestRestoredDocumentKeepsThisHostsDataDirectoryAndListener(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	document := func(dataDir string, port int, email string) []byte {
		value := Defaults()
		value.DataDir, value.Server.Port = dataDir, port
		value.Auth = Auth{Email: email, PasswordHash: testutil.ChangedPasswordHash}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	local := document("/srv/local", 3000, testutil.AdminEmail)
	restored := document("/srv/elsewhere", 8080, "backup@example.com")
	_, kept, err := RestoredDocument(path, local, restored, false)
	if err != nil || kept.DataDir != "/srv/local" || kept.Server.Port != 3000 || kept.Auth.Email != "backup@example.com" {
		t.Fatalf("kept service = %+v %+v, %v", kept.Server, kept.Auth, err)
	}
	_, taken, err := RestoredDocument(path, local, restored, true)
	if err != nil || taken.DataDir != "/srv/local" || taken.Server.Port != 8080 {
		t.Fatalf("taken service = %+v, %v", taken.Server, err)
	}
	if _, _, err := RestoredDocument(path, local, []byte(`{"server":`), false); err == nil {
		t.Fatal("invalid backup settings were accepted")
	}
}
