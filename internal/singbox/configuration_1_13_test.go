// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func reviewed113Versions(t *testing.T) []string {
	t.Helper()
	var versions []string
	for _, version := range Versions() {
		if version.SchemaSource == SchemaSourceReviewed113 {
			versions = append(versions, version.ExactVersion)
		}
	}
	if len(versions) == 0 {
		t.Fatal("no reviewed 1.13 versions in the catalog")
	}
	return versions
}

var rejected113Configurations = map[string]string{
	"wrong section type":         `{"outbounds":{}}`,
	"wrong log type":             `{"log":[]}`,
	"wrong log field type":       `{"log":{"disabled":"false"}}`,
	"invalid log level":          `{"log":{"level":"verbose"}}`,
	"unknown log field":          `{"log":{"colour":true}}`,
	"certificate providers":      `{"certificate":{"providers":[{"type":"acme","tag":"cert"}]}}`,
	"new inbound":                `{"inbounds":[{"type":"snell","listen_port":2080,"psk":"secret"}]}`,
	"new endpoint":               `{"endpoints":[{"type":"openvpn","tag":"vpn"}]}`,
	"new DNS transport":          `{"dns":{"servers":[{"type":"mdns","tag":"mdns"}]}}`,
	"new DNS response matching":  `{"dns":{"rules":[{"action":"evaluate","server":"dns"}]}}`,
	"wrong port type":            `{"inbounds":[{"type":"mixed","listen_port":"2080"}]}`,
	"invalid port":               `{"inbounds":[{"type":"mixed","listen_port":65536}]}`,
	"invalid TLS version":        `{"outbounds":[{"type":"http","tls":{"enabled":true,"min_version":"2.0"}}]}`,
	"conflicting fragment flags": `{"route":{"rules":[{"action":"route-options","tls_fragment":true,"tls_record_fragment":true}]}}`,
	"removed sniff field":        `{"inbounds":[{"type":"mixed","sniff":true}]}`,
}

func reviewed113Fixtures(t *testing.T) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob("testdata/configuration-1.13/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("configuration fixtures: %v", err)
	}
	fixtures := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fixtures[filepath.Base(path)] = data
	}
	return fixtures
}

func TestReviewed113ConfigurationContract(t *testing.T) {
	for _, version := range reviewed113Versions(t) {
		t.Run(version, func(t *testing.T) {
			for name, data := range reviewed113Fixtures(t) {
				t.Run(name, func(t *testing.T) {
					if err := ValidateConfiguration(version, data); err != nil {
						t.Fatal(err)
					}
					var decoded any
					if err := json.Unmarshal(data, &decoded); err != nil {
						t.Fatal(err)
					}
					roundTrip, err := json.Marshal(decoded)
					if err != nil {
						t.Fatal(err)
					}
					if err := ValidateConfiguration(version, roundTrip); err != nil {
						t.Fatalf("JSON round trip: %v", err)
					}
				})
			}
			for name, data := range rejected113Configurations {
				t.Run(name, func(t *testing.T) {
					if err := ValidateConfiguration(version, []byte(data)); err == nil {
						t.Fatalf("accepted unsupported configuration: %s", data)
					}
				})
			}
		})
	}
}

func TestRuntimeCompatibilityEnvironmentIsExactVersionScoped(t *testing.T) {
	for _, version := range reviewed113Versions(t) {
		environment := RuntimeCompatibilityEnvironment(version)
		if len(environment) != 5 {
			t.Fatalf("%s compatibility switches = %v", version, environment)
		}
		environment[0] = "modified"
		if RuntimeCompatibilityEnvironment(version)[0] == "modified" {
			t.Fatal("environment shares mutable backing storage")
		}
	}
	for _, version := range []string{"1.13.18", "1.13.22", "1.14.0", "v1.13.21", ""} {
		if env := RuntimeCompatibilityEnvironment(version); len(env) != 0 {
			t.Fatalf("%s received legacy switches: %v", version, env)
		}
	}
}
