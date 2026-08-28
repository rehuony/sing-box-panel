// SPDX-License-Identifier: GPL-3.0-or-later

package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
)

func TestSignVerifyAndKeyEncoding(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodePublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePublicKey(encoded)
	if err != nil {
		t.Fatal(err)
	}
	checksums := []byte("0123456789abcdef  sing-box-panel-linux-amd64\n")
	signature, err := Sign(privateKey, "v1.2.3", checksums)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(parsed, "v1.2.3", checksums, signature); err != nil {
		t.Fatal(err)
	}
	if err := Verify(parsed, "v1.2.3", append([]byte(nil), checksums[:len(checksums)-1]...), signature); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered checksum error = %v", err)
	}
	if err := Verify(parsed, "v1.2.4", checksums, signature); !errors.Is(err, ErrSignature) {
		t.Fatalf("replayed version error = %v", err)
	}
}

func TestSignAcceptsStrictPrereleaseAndBuildMetadata(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	version := "v1.2.3-rc.1+linux.amd64"
	checksums := []byte("0123456789abcdef  sing-box-panel-linux-amd64\n")
	signature, err := Sign(privateKey, version, checksums)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(publicKey, version, checksums, signature); err != nil {
		t.Fatal(err)
	}
}

func TestParsePrivateKeyAndMatch(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	if err := MatchKeyPair(publicKey, parsed); err != nil {
		t.Fatal(err)
	}

	otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := MatchKeyPair(otherPublicKey, parsed); !errors.Is(err, ErrKeyPair) {
		t.Fatalf("mismatched key error = %v", err)
	}
}

func TestRejectsMalformedKeysAndSignatures(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := Sign(privateKey, "v1.2.3", []byte("checksums\n"))
	if err != nil {
		t.Fatal(err)
	}

	for _, encoded := range []string{"", " AA==", "AA==\n", strings.Repeat("A", 44)} {
		if _, err := ParsePublicKey(encoded); !errors.Is(err, ErrPublicKey) {
			t.Errorf("ParsePublicKey(%q) error = %v", encoded, err)
		}
	}
	if _, err := ParsePrivateKey([]byte("not PEM")); !errors.Is(err, ErrPrivateKey) {
		t.Fatalf("private key error = %v", err)
	}
	for _, malformed := range [][]byte{
		nil,
		signature[:len(signature)-1],
		append(append([]byte(nil), signature...), '\n'),
		[]byte("not-base64\n"),
	} {
		if err := Verify(publicKey, "v1.2.3", []byte("checksums\n"), malformed); !errors.Is(err, ErrSignature) {
			t.Errorf("Verify(%q) error = %v", malformed, err)
		}
	}
	if _, err := Sign(privateKey, "dev", []byte("checksums\n")); !errors.Is(err, ErrVersion) {
		t.Fatalf("invalid version signing error = %v", err)
	}
}
