// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/releasesignature"
	"github.com/rehuony/sing-box-panel/internal/releaseversion"
)

func TestValidateVersionCommand(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		version string
		valid   bool
	}{
		{name: "stable", version: "v1.2.3", valid: true},
		{name: "prerelease and metadata", version: "v1.2.3-rc.1+linux.amd64", valid: true},
		{name: "missing prefix", version: "1.2.3"},
		{name: "leading zero", version: "v1.02.3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := run([]string{"validate-version", "--version", test.version}, io.Discard)
			if test.valid && err != nil {
				t.Fatalf("validate-version --version %q: %v", test.version, err)
			}
			if !test.valid && !errors.Is(err, releaseversion.ErrInvalidReleaseVersion) {
				t.Fatalf("validate-version --version %q error = %v, want ErrInvalidReleaseVersion", test.version, err)
			}
		})
	}
}

func TestSignReleaseCommandsRoundTrip(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	privateKeyPath, publicKeyPath := writeKeyPair(t, directory)
	checksumsPath := filepath.Join(directory, "SHA256SUMS")
	signaturePath := filepath.Join(directory, "SHA256SUMS.sig")
	if err := os.WriteFile(checksumsPath, []byte("digest  binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var encoded bytes.Buffer
	if err := run([]string{"public-key", "--private-key", privateKeyPath}, &encoded); err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(publicKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if encoded.String() != string(configured) {
		t.Fatalf("public key = %q, want %q", encoded.String(), configured)
	}

	if err := run([]string{
		"sign", "--private-key", privateKeyPath, "--public-key", publicKeyPath,
		"--version", "v1.2.3", "--checksums", checksumsPath, "--signature", signaturePath,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{
		"verify", "--public-key", publicKeyPath,
		"--version", "v1.2.3", "--checksums", checksumsPath, "--signature", signaturePath,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestSignReleaseRejectsMismatchedKeyWithoutOutput(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	privateKeyPath, _ := writeKeyPair(t, directory)
	_, publicKeyPath := writeKeyPair(t, filepath.Join(directory, "other"))
	checksumsPath := filepath.Join(directory, "SHA256SUMS")
	signaturePath := filepath.Join(directory, "SHA256SUMS.sig")
	if err := os.WriteFile(checksumsPath, []byte("digest  binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"sign", "--private-key", privateKeyPath, "--public-key", publicKeyPath,
		"--version", "v1.2.3", "--checksums", checksumsPath, "--signature", signaturePath,
	}, io.Discard)
	if !errors.Is(err, releasesignature.ErrKeyPair) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(signaturePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("signature stat error = %v", statErr)
	}
}

func writeKeyPair(t *testing.T, directory string) (string, string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPath := filepath.Join(directory, "private.pem")
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, err := releasesignature.EncodePublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyPath := filepath.Join(directory, "public-key")
	if err := os.WriteFile(publicKeyPath, []byte(encoded+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return privateKeyPath, publicKeyPath
}
