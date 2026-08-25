// SPDX-License-Identifier: GPL-3.0-or-later

package coreartifact

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIdentityRoundTrip(t *testing.T) {
	t.Parallel()

	source, err := NewOfficialSource(123, 456, 789)
	if err != nil {
		t.Fatalf("NewOfficialSource: %v", err)
	}
	identity, err := NewIdentity(
		source,
		mustDigest(t, strings.Repeat("ab", 32)),
		OperatingSystemLinux,
		ArchitectureARM64,
		VariantMusl,
		NewExactVersion(1, 13, 19),
	)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}

	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("Marshal identity: %v", err)
	}
	decoded, err := DecodeIdentity(encoded)
	if err != nil {
		t.Fatalf("DecodeIdentity: %v", err)
	}
	if decoded.Source().RepositoryID() != 123 || decoded.Source().ReleaseID() != 456 || decoded.Source().AssetID() != 789 {
		t.Fatalf("decoded source = %+v, want official IDs 123/456/789", decoded.Source())
	}
	if decoded.Digest() != identity.Digest() || decoded.ReportedVersion() != identity.ReportedVersion() {
		t.Fatalf("decoded identity does not match original")
	}
}

func TestIdentityValidation(t *testing.T) {
	t.Parallel()

	validDigest := mustDigest(t, strings.Repeat("01", 32))
	validVersion := NewExactVersion(1, 13, 19)
	validOfficial, err := NewOfficialSource(1, 2, 3)
	if err != nil {
		t.Fatalf("NewOfficialSource: %v", err)
	}
	validUser, err := NewUserSource("sing-box-local.tar.gz")
	if err != nil {
		t.Fatalf("NewUserSource: %v", err)
	}

	tests := []struct {
		name    string
		source  Source
		digest  SHA256
		os      OperatingSystem
		arch    Architecture
		variant Variant
		version ExactVersion
	}{
		{name: "zero official repository", source: Source{kind: SourceOfficial, releaseID: 2, assetID: 3}, digest: validDigest, os: OperatingSystemLinux, arch: ArchitectureAMD64, variant: VariantPlain, version: validVersion},
		{name: "mixed user and official", source: Source{kind: SourceUser, repositoryID: 1, userSource: "local"}, digest: validDigest, os: OperatingSystemLinux, arch: ArchitectureAMD64, variant: VariantPlain, version: validVersion},
		{name: "empty user description", source: Source{kind: SourceUser}, digest: validDigest, os: OperatingSystemLinux, arch: ArchitectureAMD64, variant: VariantPlain, version: validVersion},
		{name: "zero digest", source: validOfficial, digest: SHA256{}, os: OperatingSystemLinux, arch: ArchitectureAMD64, variant: VariantPlain, version: validVersion},
		{name: "unsupported OS", source: validOfficial, digest: validDigest, os: "darwin", arch: ArchitectureAMD64, variant: VariantPlain, version: validVersion},
		{name: "unsupported architecture", source: validUser, digest: validDigest, os: OperatingSystemLinux, arch: "386", variant: VariantPlain, version: validVersion},
		{name: "unsafe variant", source: validOfficial, digest: validDigest, os: OperatingSystemLinux, arch: ArchitectureARM64, variant: "../custom", version: validVersion},
		{name: "missing version", source: validOfficial, digest: validDigest, os: OperatingSystemLinux, arch: ArchitectureAMD64, variant: VariantGlibc, version: ExactVersion{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewIdentity(test.source, test.digest, test.os, test.arch, test.variant, test.version)
			if !errors.Is(err, ErrInvalidIdentity) {
				t.Fatalf("NewIdentity() error = %v, want ErrInvalidIdentity", err)
			}
		})
	}
}

func TestIdentityAllowsFutureSafeVariant(t *testing.T) {
	t.Parallel()

	source, err := NewUserSource("future official-compatible build")
	if err != nil {
		t.Fatalf("NewUserSource: %v", err)
	}
	if got := source.Kind(); got != SourceKind("user_verified") {
		t.Fatalf("user source kind = %q, want user_verified", got)
	}
	identity, err := NewIdentity(
		source,
		mustDigest(t, strings.Repeat("ef", 32)),
		OperatingSystemLinux,
		ArchitectureAMD64,
		Variant("future-libc_v2"),
		NewExactVersion(2, 0, 0),
	)
	if err != nil {
		t.Fatalf("NewIdentity(future variant): %v", err)
	}
	if got := identity.Variant(); got != "future-libc_v2" {
		t.Fatalf("identity.Variant() = %q, want future-libc_v2", got)
	}
}

func TestDecodeIdentityRejectsAmbiguousJSON(t *testing.T) {
	t.Parallel()

	base := `{"source_kind":"user_verified","user_source":"local","sha256":"` + strings.Repeat("12", 32) + `","os":"linux","arch":"amd64","variant":"plain","reported_version":"1.13.19"}`
	tests := []struct {
		name string
		json string
	}{
		{name: "unknown field", json: strings.Replace(base, `"os":"linux"`, `"os":"linux","path":"/tmp/core"`, 1)},
		{name: "duplicate field", json: strings.Replace(base, `"arch":"amd64"`, `"arch":"amd64","arch":"arm64"`, 1)},
		{name: "trailing value", json: base + `{}`},
		{name: "mixed source", json: strings.Replace(base, `"user_source":"local"`, `"user_source":"local","asset_id":9`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeIdentity([]byte(test.json)); !errors.Is(err, ErrInvalidIdentity) {
				t.Fatalf("DecodeIdentity(%s) error = %v, want ErrInvalidIdentity", test.name, err)
			}
		})
	}
}

func TestParseSHA256NormalizesHex(t *testing.T) {
	t.Parallel()

	upper := strings.Repeat("AB", 32)
	digest, err := ParseSHA256(upper)
	if err != nil {
		t.Fatalf("ParseSHA256: %v", err)
	}
	if got, want := digest.String(), strings.ToLower(upper); got != want {
		t.Fatalf("digest.String() = %q, want %q", got, want)
	}
	if _, err := ParseSHA256("abcd"); !errors.Is(err, ErrInvalidSHA256) {
		t.Fatalf("short digest error = %v, want ErrInvalidSHA256", err)
	}
}

func mustDigest(t *testing.T, value string) SHA256 {
	t.Helper()
	digest, err := ParseSHA256(value)
	if err != nil {
		t.Fatalf("ParseSHA256(%q): %v", value, err)
	}
	return digest
}
