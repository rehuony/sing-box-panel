// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"errors"
	"testing"
)

func TestMergeSourceSnapshotsPublishesFrozenSourceNodes(t *testing.T) {
	merged, err := MergeSourceSnapshots(
		[]byte(`{"outbounds":[{"type":"shadowsocks","tag":"base","server":"base.example","server_port":443,"method":"aes-128-gcm","password":"base"}]}`),
		[][]byte{
			[]byte(`[{"type":"vless","tag":"array-source","server":"array.example","server_port":443,"uuid":"uuid"}]`),
			[]byte(`{"outbounds":[{"type":"trojan","tag":"object-source","server":"object.example","server_port":443,"password":"secret"}]}`),
		},
	)
	if err != nil {
		t.Fatalf("MergeSourceSnapshots: %v", err)
	}
	result, err := Render(merged, Channel{Format: FormatSingBox})
	if err != nil {
		t.Fatalf("Render merged sources: %v", err)
	}
	if result.NodeCount != 3 || !bytes.Contains(result.Content, []byte(`"array-source"`)) ||
		!bytes.Contains(result.Content, []byte(`"object-source"`)) {
		t.Fatalf("merged publication = count %d body %s", result.NodeCount, result.Content)
	}
}

func TestMergeSourceSnapshotsKeepsDuplicateKeyAndCollectionValidation(t *testing.T) {
	for _, snapshot := range [][]byte{
		[]byte(`{"outbounds":[],"outbounds":[]}`),
		[]byte(`{"outbounds":{}}`),
	} {
		_, err := MergeSourceSnapshots([]byte(`{"outbounds":[]}`), [][]byte{snapshot})
		if !errors.Is(err, ErrInvalidStartup) {
			t.Fatalf("snapshot %s error = %v, want ErrInvalidStartup", snapshot, err)
		}
	}
}
