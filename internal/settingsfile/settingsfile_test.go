// SPDX-License-Identifier: GPL-3.0-or-later

package settingsfile

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

type orderedSection struct {
	Second int    `json:"second"`
	First  string `json:"first"`
}

type orderedSettings struct {
	Zeta     orderedSection   `json:"zeta"`
	Alpha    string           `json:"alpha"`
	Items    []orderedSection `json:"items"`
	Optional *orderedSection  `json:"optional"`
}

// TestSetWritesTheSettingsTypesFieldOrder: a set leaves the document as a
// settings value encodes, whatever order it was stored in, so only the
// changed value differs from what the other writers produce.
func TestSetWritesTheSettingsTypesFieldOrder(t *testing.T) {
	value := orderedSettings{
		Zeta: orderedSection{Second: 2, First: "a"}, Alpha: "x",
		Items: []orderedSection{{Second: 1, First: "i"}}, Optional: &orderedSection{Second: 3, First: "o"},
	}
	encode := func(value any) []byte {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return append(data, '\n')
	}
	field, err := FieldOf(reflect.TypeFor[orderedSettings](), "zeta.first")
	if err != nil {
		t.Fatal(err)
	}
	changed := value
	changed.Zeta.First = "b"
	want := encode(changed)

	// A sorted document is what earlier config commands wrote.
	var sorted map[string]any
	if err := json.Unmarshal(encode(value), &sorted); err != nil {
		t.Fatal(err)
	}
	for name, stored := range map[string][]byte{"field order": encode(value), "sorted": encode(sorted)} {
		got, err := field.Set(stored, json.RawMessage(`"b"`))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: set =\n%s, %v\nwant\n%s", name, got, err, want)
		}
	}

	// Fields the file leaves out stay out.
	got, err := field.Set([]byte(`{"alpha":"x","zeta":{"first":"a"}}`), json.RawMessage(`"b"`))
	if want := "{\n  \"zeta\": {\n    \"first\": \"b\"\n  },\n  \"alpha\": \"x\"\n}\n"; err != nil || string(got) != want {
		t.Errorf("sparse set = %q, %v, want %q", got, err, want)
	}
}
