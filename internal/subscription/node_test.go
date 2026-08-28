// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import "testing"

func TestPublicationDocumentRejectsDuplicateNodeIdentity(t *testing.T) {
	value := Node{Key: "local:key", SourceID: "local", Type: "socks", Tag: "local", Outbound: []byte(`{"type":"socks","tag":"local"}`)}
	if _, err := PublicationDocument([]Node{value, value}); err == nil {
		t.Fatal("PublicationDocument() accepted duplicate node identity")
	}
}
