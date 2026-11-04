// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

func TestInstallCoreRecordsTheVerifiedSourceIdentityOnce(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	application := newApplication(database)
	version, _ := coreartifact.ParseExactVersion("1.13.19")
	for _, source := range []coreartifact.Source{officialSource(t), userSource(t)} {
		identity, err := coreartifact.NewIdentity(source, testDigest(t, "ab"), coreartifact.OperatingSystemLinux,
			coreartifact.ArchitectureAMD64, coreartifact.VariantMusl, version)
		if err != nil {
			t.Fatal(err)
		}
		application.SetArtifactInstaller(fakeArtifactInstaller{result: artifacts.Result{
			Identity: identity, BinarySHA256: testDigest(t, "cd"), BinaryPath: "/secure/artifacts/sing-box",
			FeatureFingerprint: artifacts.FeatureFingerprint{Status: artifacts.FeatureFingerprintReported, Features: []string{"with_utls", "with_quic", "with_utls"}},
		}})
		candidate := artifacts.Candidate{
			Source: source, SHA256: testDigest(t, "ab"), Size: 1234, Version: version,
			Architecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantMusl,
			Write: func(context.Context, io.Writer) (int64, error) { return 0, nil },
		}
		first, err := application.InstallCore(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		second, err := application.InstallCore(ctx, candidate)
		if err != nil || second.ID != first.ID {
			t.Fatalf("repeated install = %+v, %v; want %s", second, err, first.ID)
		}
		switch source.Kind() {
		case coreartifact.SourceOfficial:
			if first.SourceKind != store.CoreArtifactSourceOfficial || first.AssetID != source.AssetID() {
				t.Fatalf("official core recorded as %+v", first)
			}
		case coreartifact.SourceUser:
			if first.SourceKind != store.CoreArtifactSourceUserVerified || first.UserSource != source.UserSource() {
				t.Fatalf("user-verified core recorded as %+v", first)
			}
		}
		if string(first.FeatureFingerprint) != `{"status":"reported","features":["with_quic","with_utls"]}` {
			t.Fatalf("feature fingerprint = %s", first.FeatureFingerprint)
		}
	}
}

type fakeArtifactInstaller struct {
	result artifacts.Result
	err    error
}

func (installer fakeArtifactInstaller) Install(context.Context, artifacts.Candidate) (artifacts.Result, error) {
	return installer.result, installer.err
}

func testDigest(t *testing.T, pair string) coreartifact.SHA256 {
	t.Helper()
	digest, err := coreartifact.ParseSHA256(strings.Repeat(pair, 32))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func officialSource(t *testing.T) coreartifact.Source {
	t.Helper()
	source, err := coreartifact.NewOfficialSource(111, 2001, 3001)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func userSource(t *testing.T) coreartifact.Source {
	t.Helper()
	source, err := coreartifact.NewUserSource("administrator verified archive")
	if err != nil {
		t.Fatal(err)
	}
	return source
}
