// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrafficAccountingAlwaysReadsFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "setting.json")
	if err := os.WriteFile(path, []byte(`{"data_dir":".","traffic":{"quota_gib":7,"sample_retention_days":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewService(nil, path, Settings{}, Options{})
	quota, err := quotaOf(app.TrafficAccounting(t.Context()))
	if err != nil || quota == nil || *quota != 7 {
		t.Fatalf("bootstrap quota = %v, %v", quota, err)
	}
	if err := os.WriteFile(path, []byte(`{"data_dir":".","traffic":{"quota_gib":-1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := quotaOf(app.TrafficAccounting(t.Context())); err == nil {
		t.Fatal("invalid bootstrap quota accepted")
	}
	for _, quotaJSON := range []string{"3", "null"} {
		if err := os.WriteFile(path, []byte(`{"data_dir":".","traffic":{"quota_gib":`+quotaJSON+`}}`), 0600); err != nil {
			t.Fatal(err)
		}
		quota, err = quotaOf(app.TrafficAccounting(t.Context()))
		if err != nil || quotaJSON == "3" && (quota == nil || *quota != 3) || quotaJSON == "null" && quota != nil {
			t.Fatalf("file quota = %v, %v", quota, err)
		}
	}
}
func quotaOf(value Traffic, err error) (*int64, error) { return value.QuotaGiB, err }
