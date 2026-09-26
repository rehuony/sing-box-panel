// SPDX-License-Identifier: GPL-3.0-or-later
package configuration

import (
	"bytes"
	"testing"
)

func TestPresentationOrderPreservesValuesAndArrays(t *testing.T) {
	input := []byte(`{"future":{"z":false,"n":null,"large":900719925474099312345,"decimal":4.2000e+99},"route":{"rules":[{"outbound":"b","domain":["z","a"]},{"outbound":"a","domain":["a"]}]},"outbounds":[{"server_port":443,"tag":"b","type":"socks","server":"host"}],"log":{"level":"info"}}`)
	actual, err := FormatJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "log": {
    "level": "info"
  },
  "outbounds": [
    {
      "type": "socks",
      "tag": "b",
      "server": "host",
      "server_port": 443
    }
  ],
  "route": {
    "rules": [
      {
        "domain": [
          "z",
          "a"
        ],
        "outbound": "b"
      },
      {
        "domain": [
          "a"
        ],
        "outbound": "a"
      }
    ]
  },
  "future": {
    "decimal": 4.2000e+99,
    "large": 900719925474099312345,
    "n": null,
    "z": false
  }
}`
	if string(actual) != want {
		t.Fatalf("unexpected presentation:\n%s", actual)
	}
	again, err := FormatJSON(actual)
	if err != nil || !bytes.Equal(actual, again) {
		t.Fatal("formatting is not idempotent", err)
	}
	if _, err := FormatJSON([]byte(`{"log":`)); err == nil {
		t.Fatal("incomplete JSON accepted")
	}
}
