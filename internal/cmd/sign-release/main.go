// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/releasesignature"
	"github.com/rehuony/sing-box-panel/internal/releaseversion"
)

const (
	maxKeyBytes       = 16 << 10
	maxChecksumsBytes = 1 << 20
	maxSignatureBytes = 1 << 10
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: sign-release public-key|sign|verify|validate-version [options]")
	}
	switch arguments[0] {
	case "public-key":
		return runPublicKey(arguments[1:], stdout)
	case "sign":
		return runSign(arguments[1:])
	case "verify":
		return runVerify(arguments[1:])
	case "validate-version":
		return runValidateVersion(arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func runValidateVersion(arguments []string) error {
	flags := flag.NewFlagSet("validate-version", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	version := flags.String("version", "", "strict v-prefixed SemVer release version")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *version == "" {
		return errors.New("usage: sign-release validate-version --version vX.Y.Z")
	}
	return releaseversion.Validate(*version)
}

func runPublicKey(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("public-key", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	privateKeyPath := flags.String("private-key", "", "PKCS#8 PEM private key")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *privateKeyPath == "" {
		return errors.New("usage: sign-release public-key --private-key FILE")
	}
	privateKey, err := readPrivateKey(*privateKeyPath)
	if err != nil {
		return err
	}
	publicKey, err := releasesignature.PublicKey(privateKey)
	if err != nil {
		return err
	}
	encoded, err := releasesignature.EncodePublicKey(publicKey)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, encoded)
	return err
}

func runSign(arguments []string) error {
	flags := flag.NewFlagSet("sign", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	privateKeyPath := flags.String("private-key", "", "PKCS#8 PEM private key")
	publicKeyPath := flags.String("public-key", "", "Base64 public key")
	version := flags.String("version", "", "strict v-prefixed SemVer release version")
	checksumsPath := flags.String("checksums", "", "checksum manifest")
	signaturePath := flags.String("signature", "", "detached signature output")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 ||
		*privateKeyPath == "" || *publicKeyPath == "" || *version == "" || *checksumsPath == "" || *signaturePath == "" {
		return errors.New("usage: sign-release sign --private-key FILE --public-key FILE --version vX.Y.Z --checksums FILE --signature FILE")
	}
	privateKey, err := readPrivateKey(*privateKeyPath)
	if err != nil {
		return err
	}
	publicKey, err := readPublicKey(*publicKeyPath)
	if err != nil {
		return err
	}
	if err := releasesignature.MatchKeyPair(publicKey, privateKey); err != nil {
		return err
	}
	checksums, err := readRegularFile(*checksumsPath, maxChecksumsBytes, false)
	if err != nil {
		return err
	}
	signature, err := releasesignature.Sign(privateKey, *version, checksums)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*signaturePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create signature: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(*signaturePath)
		}
	}()
	if _, err := file.Write(signature); err != nil {
		return fmt.Errorf("write signature: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync signature: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close signature: %w", err)
	}
	remove = false
	return nil
}

func runVerify(arguments []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	publicKeyPath := flags.String("public-key", "", "Base64 public key")
	version := flags.String("version", "", "strict v-prefixed SemVer release version")
	checksumsPath := flags.String("checksums", "", "checksum manifest")
	signaturePath := flags.String("signature", "", "detached signature")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 ||
		*publicKeyPath == "" || *version == "" || *checksumsPath == "" || *signaturePath == "" {
		return errors.New("usage: sign-release verify --public-key FILE --version vX.Y.Z --checksums FILE --signature FILE")
	}
	publicKey, err := readPublicKey(*publicKeyPath)
	if err != nil {
		return err
	}
	checksums, err := readRegularFile(*checksumsPath, maxChecksumsBytes, false)
	if err != nil {
		return err
	}
	signature, err := readRegularFile(*signaturePath, maxSignatureBytes, false)
	if err != nil {
		return err
	}
	return releasesignature.Verify(publicKey, *version, checksums, signature)
}

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := readRegularFile(path, maxKeyBytes, true)
	if err != nil {
		return nil, err
	}
	key, err := releasesignature.ParsePrivateKey(data)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func readPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readRegularFile(path, maxKeyBytes, false)
	if err != nil {
		return nil, err
	}
	return releasesignature.ParsePublicKey(string(bytesTrimSingleNewline(data)))
}

func readRegularFile(path string, maximum int64, rejectSymlink bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() || rejectSymlink && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a regular file", filepath.Base(path))
	}
	if info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("%s has an invalid size", filepath.Base(path))
	}
	return os.ReadFile(path)
}

func bytesTrimSingleNewline(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\n' {
		return data[:len(data)-1]
	}
	return data
}
