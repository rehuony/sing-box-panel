// SPDX-License-Identifier: GPL-3.0-or-later

package canonical

import (
	"errors"
	"testing"
)

func TestCanonicalPointerGetSetAndUnset(t *testing.T) {
	document := Empty()
	updated, err := document.SetPointer("/global/log~1level", "warn")
	if err != nil {
		t.Fatal(err)
	}
	value, err := updated.ValueAtPointer("/global/log~1level")
	if err != nil || value != "warn" {
		t.Fatalf("value=%#v err=%v", value, err)
	}
	removed, err := updated.UnsetPointer("/global/log~1level")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := removed.ValueAtPointer("/global/log~1level"); !errors.Is(err, ErrPointerNotFound) {
		t.Fatalf("missing value error = %v", err)
	}
}

func TestCanonicalPointerArrayRemovalAndValidation(t *testing.T) {
	document, err := Parse([]byte(`{"schema_version":1,"global":{},"nodes":[{"id":"a","kind":"outbound","enabled":true},{"id":"b","kind":"outbound","enabled":true}],"rules":[],"subscription":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := document.UnsetPointer("/nodes/0")
	if err != nil {
		t.Fatal(err)
	}
	value, err := updated.ValueAtPointer("/nodes/0/id")
	if err != nil || value != "b" {
		t.Fatalf("shifted node id=%#v err=%v", value, err)
	}
	if _, err := updated.SetPointer("nodes/0", true); err == nil {
		t.Fatal("invalid pointer succeeded")
	}
	if _, err := updated.UnsetPointer("/schema_version"); err == nil {
		t.Fatal("removing required canonical field succeeded")
	}
}
