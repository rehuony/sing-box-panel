// SPDX-License-Identifier: GPL-3.0-or-later

// Package nodeapi holds the generated panel–node protocol code (sbp.node.v1),
// its protocol version, the WebSocket transport that carries its frames and
// the helpers both services use to build them.
package nodeapi

import (
	"crypto/ed25519"
	"encoding/binary"
)

// The protocol version this build speaks. The major version names the
// protobuf package; the minor version grows with additive changes.
const (
	ProtocolMajor = 1
	ProtocolMinor = 3
)

// MaximumCoreSchemaBytes bounds the schema a node sends in CoreSchemaOutcome.
// It leaves room for the result around it within MaximumFrameBytes.
const MaximumCoreSchemaBytes = 3 << 20

// NonceSize is the length of the random nonce in Challenge.
const NonceSize = 32

// Proof contexts bind a node's signature to one handshake of this protocol,
// so a node key never signs anything that verifies in another context: a
// session proof cannot open FetchRelease and a release proof cannot open a
// session.
const (
	proofContext        = "sing-box-panel node session proof v1"
	releaseProofContext = "sing-box-panel node release proof v1"
)

// ProofMessage returns the bytes a node signs to answer a session challenge:
// the context string, the panel ID, the node ID and the nonce, each prefixed
// with its length so that no two inputs encode to the same message.
func ProofMessage(panelID, nodeID string, nonce []byte) []byte {
	return proofMessage(proofContext, panelID, nodeID, nonce)
}

// ReleaseProofMessage is the message a node signs to answer a FetchRelease
// challenge.
func ReleaseProofMessage(panelID, nodeID string, nonce []byte) []byte {
	return proofMessage(releaseProofContext, panelID, nodeID, nonce)
}

func proofMessage(context, panelID, nodeID string, nonce []byte) []byte {
	message := make([]byte, 0, 4*4+len(context)+len(panelID)+len(nodeID)+len(nonce))
	for _, field := range [][]byte{[]byte(context), []byte(panelID), []byte(nodeID), nonce} {
		message = binary.BigEndian.AppendUint32(message, uint32(len(field)))
		message = append(message, field...)
	}
	return message
}

// SignProof answers a session challenge with the node's identity key.
func SignProof(key ed25519.PrivateKey, panelID, nodeID string, nonce []byte) *Proof {
	return &Proof{Signature: ed25519.Sign(key, ProofMessage(panelID, nodeID, nonce))}
}

// SignReleaseProof answers a FetchRelease challenge with the node's identity
// key.
func SignReleaseProof(key ed25519.PrivateKey, panelID, nodeID string, nonce []byte) *Proof {
	return &Proof{Signature: ed25519.Sign(key, ReleaseProofMessage(panelID, nodeID, nonce))}
}

// VerifyProof reports whether proof answers the session challenge for nodeID
// with the enrolled public key.
func VerifyProof(publicKey ed25519.PublicKey, panelID, nodeID string, nonce []byte, proof *Proof) bool {
	return verify(publicKey, ProofMessage(panelID, nodeID, nonce), nonce, proof)
}

// VerifyReleaseProof reports whether proof answers the FetchRelease
// challenge for nodeID with the enrolled public key.
func VerifyReleaseProof(publicKey ed25519.PublicKey, panelID, nodeID string, nonce []byte, proof *Proof) bool {
	return verify(publicKey, ReleaseProofMessage(panelID, nodeID, nonce), nonce, proof)
}

func verify(publicKey ed25519.PublicKey, message, nonce []byte, proof *Proof) bool {
	if len(publicKey) != ed25519.PublicKeySize || len(nonce) != NonceSize || proof == nil {
		return false
	}
	return ed25519.Verify(publicKey, message, proof.GetSignature())
}

// ResultOutcome is one outcome of a Result, for code that builds results.
type ResultOutcome = isResult_Outcome

// CommandBody is one body of a Command, for code that builds commands.
type CommandBody = isCommand_Body

// ChangesState reports whether a command body can change the node's state.
// The node records the results of such commands under their request IDs, and
// the panel lets them finish even when the client that asked goes away.
func ChangesState(body CommandBody) bool {
	switch body.(type) {
	case *Command_GetRuntimeStatus, *Command_ListCores, *Command_GetCoreSchema,
		*Command_BrowseFilesystem, *Command_ResolveFilesystemPath,
		*Command_ListCoreLogs, *Command_ReadCoreLog:
		return false
	}
	return true
}
