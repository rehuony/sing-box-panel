// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDataDirIgnoresUnrelatedRuntimeFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	for _, input := range []string{
		`{"data_dir":"data","traffic":{"sample_retention_days":0}}`,
		`{"data_dir":"data","server":{"port":"invalid"},"traffic":false,"future_field":true}`,
	} {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadDataDir(path)
		if err != nil || got != filepath.Join(filepath.Dir(path), "data") {
			t.Fatalf("LoadDataDir() = %q, %v", got, err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("full runtime settings accepted invalid fields")
		}
	}
}

func TestLoadDataDirRejectsAmbiguousOrMissingLocations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	for _, input := range []string{
		`{`, `{}`, `null`, `[]`, `{"data_dir":null}`, `{"data_dir":42}`,
		`{"data_dir":""}`, `{"data_dir":"  "}`, `{"data_dir":"a\u0000b"}`,
		`{"data_dir":"one","data_dir":"two"}`,
		`{"data_dir":"data","traffic":{"quota_gib":1,"quota_gib":2}}`,
		`{"data_dir":"data"} {}`,
		`{"data_dir":"` + strings.Repeat("x", maxSettingsBytes) + `"}`,
	} {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDataDir(path); err == nil {
			t.Fatalf("LoadDataDir accepted invalid location (input length %d)", len(input))
		}
	}
}

func TestLoadTrafficQuotaValidatesOnlyTheRequestedPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	for _, test := range []struct {
		input string
		want  int64
		nil   bool
		bad   bool
	}{
		{input: `{}`, nil: true},
		{input: `{"traffic":{"quota_gib":null}}`, nil: true},
		{input: `{"traffic":{"quota_gib":0}}`},
		{input: `{"traffic":{"quota_gib":7,"sample_retention_days":0},"server":false}`, want: 7},
		{input: `{"traffic":{"quota_gib":-1}}`, bad: true},
		{input: `{"traffic":{"quota_gib":8589934592}}`, bad: true},
		{input: `{"traffic":{"quota_gib":"invalid"}}`, bad: true},
		{input: `{"traffic":false}`, bad: true},
	} {
		if err := os.WriteFile(path, []byte(test.input), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadTrafficQuota(path)
		if test.bad {
			if err == nil {
				t.Fatalf("accepted invalid quota: %s", test.input)
			}
			continue
		}
		if err != nil || (got == nil) != test.nil || got != nil && *got != test.want {
			t.Fatalf("LoadTrafficQuota(%s) = %v, %v", test.input, got, err)
		}
	}
}
