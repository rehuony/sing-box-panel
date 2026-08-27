// SPDX-License-Identifier: GPL-3.0-or-later

// Package releasesignature signs and verifies sing-box-panel release versions
// and checksum manifests.
package releasesignature

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/releaseversion"
)

const messageDomain = "sing-box-panel release checksums v1\n"

var (
	ErrPublicKey  = errors.New("invalid Ed25519 public key")
	ErrPrivateKey = errors.New("invalid Ed25519 private key")
	ErrVersion    = errors.New("invalid signed release version")
	ErrSignature  = errors.New("invalid release signature")
	ErrKeyPair    = errors.New("release signing keys do not match")
)

// ParsePublicKey decodes a single standard-Base64 Ed25519 public key.
func ParsePublicKey(encoded string) (ed25519.PublicKey, error) {
	if encoded == "" || encoded != strings.TrimSpace(encoded) || strings.ContainsAny(encoded, "\r\n\t ") {
		return nil, ErrPublicKey
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, ErrPublicKey
	}
	return ed25519.PublicKey(decoded), nil
}

// EncodePublicKey returns the canonical representation accepted by
// ParsePublicKey.
func EncodePublicKey(key ed25519.PublicKey) (string, error) {
	if len(key) != ed25519.PublicKeySize {
		return "", ErrPublicKey
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// ParsePrivateKey decodes an unencrypted PKCS#8 PEM Ed25519 private key.
func ParsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, ErrPrivateKey
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateKey, err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(key) != ed25519.PrivateKeySize {
		return nil, ErrPrivateKey
	}
	return key, nil
}

// PublicKey returns the canonical public key corresponding to privateKey.
func PublicKey(privateKey ed25519.PrivateKey) (ed25519.PublicKey, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, ErrPrivateKey
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, ErrPrivateKey
	}
	return publicKey, nil
}

// Sign returns a canonical, newline-terminated detached signature file that
// binds version to the exact checksum-manifest bytes.
func Sign(privateKey ed25519.PrivateKey, version string, checksums []byte) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, ErrPrivateKey
	}
	message, err := signedMessage(version, checksums)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(privateKey, message)
	encoded := base64.StdEncoding.EncodeToString(signature)
	return append([]byte(encoded), '\n'), nil
}

// Verify validates a canonical detached signature file over version and
// checksums.
func Verify(publicKey ed25519.PublicKey, version string, checksums, signatureFile []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return ErrPublicKey
	}
	message, err := signedMessage(version, checksums)
	if err != nil {
		return err
	}
	if len(signatureFile) == 0 || signatureFile[len(signatureFile)-1] != '\n' ||
		bytes.Count(signatureFile, []byte{'\n'}) != 1 {
		return ErrSignature
	}
	encoded := string(signatureFile[:len(signatureFile)-1])
	if encoded == "" || strings.ContainsAny(encoded, "\r\n\t ") {
		return ErrSignature
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrSignature
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return ErrSignature
	}
	return nil
}

// MatchKeyPair rejects a private key that does not correspond to the public
// key configured for release builds.
func MatchKeyPair(publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) error {
	derived, err := PublicKey(privateKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(publicKey, derived) {
		return ErrKeyPair
	}
	return nil
}

func signedMessage(version string, checksums []byte) ([]byte, error) {
	if err := releaseversion.Validate(version); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVersion, err)
	}
	message := make([]byte, 0, len(messageDomain)+len(version)+1+len(checksums))
	message = append(message, messageDomain...)
	message = append(message, version...)
	message = append(message, '\n')
	message = append(message, checksums...)
	return message, nil
}
