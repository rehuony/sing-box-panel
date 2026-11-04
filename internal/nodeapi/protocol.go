// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodeapi holds the generated panel–node protocol code (sbp.node.v1),
// its protocol version and the helpers both services use to build frames.
package nodeapi

import (
	"crypto/ed25519"
	"encoding/binary"
)

// The protocol version this build speaks. The major version names the
// protobuf package; the minor version grows with additive changes.
const (
	ProtocolMajor = 1
	ProtocolMinor = 0
)

// NonceSize is the length of the random nonce in Challenge.
const NonceSize = 32

// proofContext binds a signature to session authentication of this protocol,
// so a node key never signs anything that verifies in another context.
const proofContext = "sing-box-panel node session proof v1"

// ProofMessage returns the bytes a node signs to answer a challenge: the
// context string, the panel ID, the node ID and the nonce, each prefixed with
// its length so that no two inputs encode to the same message.
func ProofMessage(panelID, nodeID string, nonce []byte) []byte {
	message := make([]byte, 0, 4*4+len(proofContext)+len(panelID)+len(nodeID)+len(nonce))
	for _, field := range [][]byte{[]byte(proofContext), []byte(panelID), []byte(nodeID), nonce} {
		message = binary.BigEndian.AppendUint32(message, uint32(len(field)))
		message = append(message, field...)
	}
	return message
}

// SignProof answers a challenge with the node's identity key.
func SignProof(key ed25519.PrivateKey, panelID, nodeID string, nonce []byte) *Proof {
	return &Proof{Signature: ed25519.Sign(key, ProofMessage(panelID, nodeID, nonce))}
}

// VerifyProof reports whether proof answers the challenge for nodeID with the
// enrolled public key.
func VerifyProof(publicKey ed25519.PublicKey, panelID, nodeID string, nonce []byte, proof *Proof) bool {
	if len(publicKey) != ed25519.PublicKeySize || len(nonce) != NonceSize || proof == nil {
		return false
	}
	return ed25519.Verify(publicKey, ProofMessage(panelID, nodeID, nonce), proof.GetSignature())
}

// ResultOutcome is one outcome of a Result, for code that builds results.
type ResultOutcome = isResult_Outcome

// CommandBody is one body of a Command, for code that builds commands.
type CommandBody = isCommand_Body
