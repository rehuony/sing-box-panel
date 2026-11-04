// SPDX-License-Identifier: GPL-3.0-or-later

package nodeapi

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestProofBindsPanelNodeAndNonce(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{7}, NonceSize)
	proof := SignProof(privateKey, "panel-a", "node-a", nonce)
	if !VerifyProof(publicKey, "panel-a", "node-a", nonce, proof) {
		t.Fatal("valid proof was rejected")
	}
	otherNonce := bytes.Repeat([]byte{8}, NonceSize)
	otherKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, verify := range map[string]func() bool{
		"panel": func() bool { return VerifyProof(publicKey, "panel-b", "node-a", nonce, proof) },
		"node":  func() bool { return VerifyProof(publicKey, "panel-a", "node-b", nonce, proof) },
		"nonce": func() bool { return VerifyProof(publicKey, "panel-a", "node-a", otherNonce, proof) },
		"key":   func() bool { return VerifyProof(otherKey, "panel-a", "node-a", nonce, proof) },
		"short": func() bool { return VerifyProof(publicKey, "panel-a", "node-a", nonce[:16], proof) },
	} {
		if verify() {
			t.Errorf("proof verified with a different %s", name)
		}
	}
	// Length prefixes keep field boundaries unambiguous.
	if bytes.Equal(ProofMessage("ab", "c", nonce), ProofMessage("a", "bc", nonce)) {
		t.Fatal("different identifiers produced the same proof message")
	}
}
