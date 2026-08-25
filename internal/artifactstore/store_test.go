// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/catalog"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestInstallOfficialVerifiesAndPublishesContentAddressedBinary(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	binary := minimalELF(coreartifact.ArchitectureAMD64)
	archive := makeArchive(t,
		tarEntry{name: "sing-box-1.13.19-linux-amd64/", kind: tar.TypeDir},
		tarEntry{name: "sing-box-1.13.19-linux-amd64/LICENSE", data: []byte("license"), kind: tar.TypeReg},
		tarEntry{name: "sing-box-1.13.19-linux-amd64/sing-box", data: binary, kind: tar.TypeReg},
	)
	digest := bytesDigest(archive)
	downloader := &memoryDownloader{data: archive}
	fingerprint, err := newReportedFeatureFingerprint([]string{"with_utls", "with_quic"})
	if err != nil {
		t.Fatal(err)
	}
	inspector := &fakeInspector{report: VersionReport{Version: version, FeatureFingerprint: fingerprint}}
	store := newTestStore(t, downloader, inspector, Limits{})
	asset := officialAsset(version, digest, int64(len(archive)))

	result, err := store.InstallOfficial(context.Background(), asset)
	if err != nil {
		t.Fatalf("InstallOfficial: %v", err)
	}
	if result.Identity.Source().Kind() != coreartifact.SourceOfficial || result.Identity.Source().RepositoryID() != catalog.OfficialRepositoryID ||
		result.Identity.Source().ReleaseID() != 11 || result.Identity.Source().AssetID() != 13 {
		t.Fatalf("installed identity source = %+v", result.Identity.Source())
	}
	if result.Identity.Digest() != digest || result.Identity.ReportedVersion() != version {
		t.Fatalf("installed identity = %+v", result.Identity)
	}
	featureJSON, err := result.FeatureFingerprint.CanonicalJSON()
	if err != nil || string(featureJSON) != `{"status":"reported","features":["with_quic","with_utls"]}` {
		t.Fatalf("feature fingerprint = %s, err=%v", featureJSON, err)
	}
	wantPath := filepath.Join(store.Root(), "sha256", digest.String()[:2], digest.String(), "sing-box")
	if result.BinaryPath != wantPath {
		t.Fatalf("BinaryPath = %q, want %q", result.BinaryPath, wantPath)
	}
	wantArchivePath := filepath.Join(store.Root(), "sha256", digest.String()[:2], digest.String(), "artifact.tar.gz")
	if result.ArchivePath != wantArchivePath {
		t.Fatalf("ArchivePath = %q, want %q", result.ArchivePath, wantArchivePath)
	}
	storedArchive, err := os.ReadFile(result.ArchivePath)
	if err != nil || !bytes.Equal(storedArchive, archive) {
		t.Fatalf("stored archive mismatch: %v", err)
	}
	archiveInfo, err := os.Stat(result.ArchivePath)
	if err != nil {
		t.Fatalf("Stat(stored archive): %v", err)
	}
	if archiveInfo.Mode().Perm() != 0o600 {
		t.Fatalf("stored archive mode = %v, want 0600", archiveInfo.Mode().Perm())
	}
	installed, err := os.ReadFile(result.BinaryPath)
	if err != nil {
		t.Fatalf("ReadFile(installed): %v", err)
	}
	if !bytes.Equal(installed, binary) {
		t.Fatalf("installed binary differs from archive binary")
	}
	info, err := os.Stat(result.BinaryPath)
	if err != nil {
		t.Fatalf("Stat(installed): %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("installed mode = %o, want 0700", info.Mode().Perm())
	}
	if len(result.Diagnostics) != 7 || downloader.calls != 1 || inspector.calls != 1 {
		t.Fatalf("result diagnostics/calls = %d/%d/%d", len(result.Diagnostics), downloader.calls, inspector.calls)
	}

	second, err := store.InstallOfficial(context.Background(), asset)
	if err != nil {
		t.Fatalf("InstallOfficial(existing): %v", err)
	}
	if second.BinaryPath != result.BinaryPath {
		t.Fatalf("second path = %q, want %q", second.BinaryPath, result.BinaryPath)
	}
}

func TestInstallDetectsMutatedContentAddress(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	digest := bytesDigest(archive)
	store := newTestStore(t, &memoryDownloader{data: archive}, &fakeInspector{report: VersionReport{Version: version}}, Limits{})
	result, err := store.InstallOfficial(context.Background(), officialAsset(version, digest, int64(len(archive))))
	if err != nil {
		t.Fatalf("InstallOfficial: %v", err)
	}
	if err := os.WriteFile(result.BinaryPath, []byte("mutated"), 0o700); err != nil {
		t.Fatalf("mutate fixture: %v", err)
	}
	_, err = store.InstallOfficial(context.Background(), officialAsset(version, digest, int64(len(archive))))
	if !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("InstallOfficial(mutated store) error = %v, want ErrCorruptStore", err)
	}
}

func TestInstallPublishesFreshArchiveBytesAfterVersionInspection(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	binary := minimalELF(coreartifact.ArchitectureAMD64)
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: binary, kind: tar.TypeReg})
	store := newTestStore(t, &memoryDownloader{data: archive}, inspectorFunc(func(_ context.Context, path string, _ int64) (VersionReport, error) {
		if err := os.WriteFile(path, []byte("replacement payload"), 0o700); err != nil {
			return VersionReport{}, err
		}
		return VersionReport{Version: version}, nil
	}), Limits{})
	result, err := store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(archive), int64(len(archive))))
	if err != nil {
		t.Fatalf("InstallOfficial: %v", err)
	}
	installed, err := os.ReadFile(result.BinaryPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(installed, binary) {
		t.Fatalf("published binary was influenced by the executed copy")
	}
}

func TestInstallRejectsSymlinkedContentStoreDirectory(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	root := filepath.Join(base, "artifacts")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("Mkdir(root): %v", err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatalf("Mkdir(outside): %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sha256")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	version := artifactVersion(t, "1.13.19")
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	store, err := New(Options{
		Root: root, Downloader: &memoryDownloader{data: archive},
		Inspector: &fakeInspector{report: VersionReport{Version: version}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(archive), int64(len(archive))))
	if !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("InstallOfficial(symlinked store) error = %v, want ErrCorruptStore", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("ReadDir(outside): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target received %d entries", len(entries))
	}
}

func TestNewRejectsWorldWritableNonStickyRootParent(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	_, err := New(Options{Root: filepath.Join(parent, "artifacts"), Downloader: &memoryDownloader{}, Inspector: &fakeInspector{}})
	if !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("New(untrusted parent) error = %v, want ErrCorruptStore", err)
	}
}

func TestInstallDoesNotTrustDownloaderReportedSize(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	downloader := downloaderFunc(func(_ context.Context, _ string, destination io.Writer, _ int64) (int64, error) {
		written, err := destination.Write(archive)
		return int64(written + 1), err
	})
	store := newTestStore(t, downloader, &fakeInspector{report: VersionReport{Version: version}}, Limits{})
	_, err := store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(archive), int64(len(archive))))
	if !errors.Is(err, ErrDigest) {
		t.Fatalf("InstallOfficial(lying downloader) error = %v, want ErrDigest", err)
	}
}

func TestInstallOfficialRejectsDigestEvidenceBeforeDownload(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	first := bytesDigest([]byte("first"))
	second := bytesDigest([]byte("second"))
	tests := []struct {
		name  string
		asset catalog.Asset
	}{
		{name: "missing", asset: officialAsset(version, coreartifact.SHA256{}, 100)},
		{name: "mismatch", asset: func() catalog.Asset {
			asset := officialAsset(version, first, 100)
			asset.CatalogDigest, asset.HasCatalogDigest = second, true
			return asset
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			downloader := &memoryDownloader{data: []byte("must not download")}
			store := newTestStore(t, downloader, &fakeInspector{}, Limits{})
			if _, err := store.InstallOfficial(context.Background(), test.asset); err == nil {
				t.Fatalf("InstallOfficial() succeeded without trustworthy digest evidence")
			}
			if downloader.calls != 0 {
				t.Fatalf("downloader calls = %d, want 0", downloader.calls)
			}
		})
	}
}

func TestInstallRejectsDigestSizeELFAndVersionMismatch(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	validArchive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	tests := []struct {
		name      string
		data      []byte
		assetSize int64
		digest    coreartifact.SHA256
		inspector VersionInspector
		want      error
	}{
		{name: "digest", data: validArchive, assetSize: int64(len(validArchive)), digest: bytesDigest([]byte("different")), inspector: &fakeInspector{report: VersionReport{Version: version}}, want: ErrDigest},
		{name: "declared size", data: validArchive, assetSize: int64(len(validArchive) + 1), digest: bytesDigest(validArchive), inspector: &fakeInspector{report: VersionReport{Version: version}}, want: ErrDigest},
		{name: "ELF architecture", data: makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureARM64), kind: tar.TypeReg}), inspector: &fakeInspector{report: VersionReport{Version: version}}, want: ErrELF},
		{name: "reported version", data: validArchive, inspector: &fakeInspector{report: VersionReport{Version: artifactVersion(t, "1.12.0")}}, want: ErrVersion},
		{name: "feature fingerprint", data: validArchive, inspector: &fakeInspector{report: VersionReport{
			Version: version,
			FeatureFingerprint: FeatureFingerprint{
				Status: FeatureFingerprintReported, Features: []string{"with_quic;unexpected"},
			},
		}}, want: ErrVersion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.digest.IsZero() {
				test.digest = bytesDigest(test.data)
			}
			if test.assetSize == 0 {
				test.assetSize = int64(len(test.data))
			}
			store := newTestStore(t, &memoryDownloader{data: test.data}, test.inspector, Limits{})
			asset := officialAsset(version, test.digest, test.assetSize)
			_, err := store.InstallOfficial(context.Background(), asset)
			if !errors.Is(err, test.want) {
				t.Fatalf("InstallOfficial() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestInstallRejectsUnsafeArchives(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	tests := []struct {
		name    string
		entries []tarEntry
	}{
		{name: "traversal", entries: []tarEntry{{name: "../sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg}}},
		{name: "symlink", entries: []tarEntry{{name: "bundle/sing-box", kind: tar.TypeSymlink, link: "/bin/sh"}}},
		{name: "hardlink", entries: []tarEntry{{name: "bundle/sing-box", kind: tar.TypeLink, link: "other"}}},
		{name: "duplicate binary", entries: []tarEntry{
			{name: "one/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg},
			{name: "two/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg},
		}},
		{name: "missing binary", entries: []tarEntry{{name: "bundle/readme", data: []byte("none"), kind: tar.TypeReg}}},
		{name: "non-canonical path", entries: []tarEntry{{name: "bundle/../sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			archive := makeArchive(t, test.entries...)
			store := newTestStore(t, &memoryDownloader{data: archive}, &fakeInspector{report: VersionReport{Version: version}}, Limits{})
			_, err := store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(archive), int64(len(archive))))
			if !errors.Is(err, ErrArchive) {
				t.Fatalf("InstallOfficial() error = %v, want ErrArchive", err)
			}
		})
	}
}

func TestInstallRejectsExpandedArchiveLimit(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	limits := DefaultLimits()
	limits.MaximumFileBytes = 32
	limits.MaximumExpandedBytes = 64
	store := newTestStore(t, &memoryDownloader{data: archive}, &fakeInspector{report: VersionReport{Version: version}}, limits)
	_, err := store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(archive), int64(len(archive))))
	if !errors.Is(err, ErrTooLarge) || !errors.Is(err, ErrArchive) {
		t.Fatalf("expanded limit error = %v, want ErrTooLarge and ErrArchive", err)
	}
}

func TestInstallRejectsCorruptOrConcatenatedGzipStreams(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	valid := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	corruptFooter := append([]byte(nil), valid...)
	corruptFooter[len(corruptFooter)-8] ^= 0xff
	concatenated := append(append([]byte(nil), valid...), valid...)
	for _, test := range []struct {
		name    string
		archive []byte
	}{
		{name: "corrupt checksum", archive: corruptFooter},
		{name: "concatenated member", archive: concatenated},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := newTestStore(t, &memoryDownloader{data: test.archive}, &fakeInspector{report: VersionReport{Version: version}}, Limits{})
			_, err := store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(test.archive), int64(len(test.archive))))
			if !errors.Is(err, ErrArchive) {
				t.Fatalf("InstallOfficial() error = %v, want ErrArchive", err)
			}
		})
	}
}

func TestImportLocalRequiresUserDigestAndMarksIdentity(t *testing.T) {
	t.Parallel()
	version := artifactVersion(t, "1.13.19")
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	sourcePath := filepath.Join(t.TempDir(), "sing-box.tar.gz")
	if err := os.WriteFile(sourcePath, archive, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := newTestStore(t, &memoryDownloader{}, &fakeInspector{report: VersionReport{Version: version}}, Limits{})
	request := ImportRequest{
		SourcePath: sourcePath, SourceDescription: "manually verified archive",
		ExpectedSHA256: bytesDigest(archive), ExpectedVersion: version,
		ExpectedArchitecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantPlain,
	}
	result, err := store.ImportLocal(context.Background(), request)
	if err != nil {
		t.Fatalf("ImportLocal: %v", err)
	}
	if result.Identity.Source().Kind() != coreartifact.SourceUser {
		t.Fatalf("import source kind = %q, want %q", result.Identity.Source().Kind(), coreartifact.SourceUser)
	}
	request.ExpectedSHA256 = coreartifact.SHA256{}
	if _, err := store.ImportLocal(context.Background(), request); !errors.Is(err, ErrDigest) {
		t.Fatalf("ImportLocal(without digest) error = %v, want ErrDigest", err)
	}
}

func TestImportLocalRejectsSymlinkSource(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	target := filepath.Join(directory, "target.tar.gz")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	link := filepath.Join(directory, "link.tar.gz")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := newTestStore(t, &memoryDownloader{}, &fakeInspector{}, Limits{})
	_, err := store.ImportLocal(context.Background(), ImportRequest{
		SourcePath: link, SourceDescription: "link", ExpectedSHA256: bytesDigest([]byte("data")),
		ExpectedVersion: artifactVersion(t, "1.13.19"), ExpectedArchitecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantPlain,
	})
	if err == nil {
		t.Fatalf("ImportLocal accepted symlink source")
	}
}

func TestParseVersionOutputIsStrict(t *testing.T) {
	t.Parallel()
	report, err := parseVersionOutput([]byte(
		"sing-box version 1.13.19\n\n" +
			"Environment: go1.25.10 linux/amd64\n" +
			"Tags: with_utls,with_quic,with_utls,badlinkname\n" +
			"Revision: 0123456789abcdef\n" +
			"Future-Metadata: retained-for-forward-compatibility\n" +
			"CGO: disabled\n",
	))
	if err != nil || report.Version.String() != "1.13.19" {
		t.Fatalf("parseVersionOutput(valid) = (%+v, %v)", report, err)
	}
	fingerprintJSON, err := report.FeatureFingerprint.CanonicalJSON()
	if err != nil || string(fingerprintJSON) != `{"status":"reported","features":["badlinkname","with_quic","with_utls"]}` {
		t.Fatalf("reported feature fingerprint = %s, err=%v", fingerprintJSON, err)
	}

	legacy, err := parseVersionOutput([]byte(
		"sing-box version 1.1.2\n\nEnvironment: go1.19.4 linux/amd64\nCGO: enabled\n",
	))
	if err != nil || legacy.FeatureFingerprint.Status != FeatureFingerprintNotReported || len(legacy.FeatureFingerprint.Features) != 0 {
		t.Fatalf("legacy version report = %+v, err=%v", legacy, err)
	}
	legacyJSON, err := legacy.FeatureFingerprint.CanonicalJSON()
	if err != nil || string(legacyJSON) != `{"status":"not_reported"}` {
		t.Fatalf("legacy feature fingerprint = %s, err=%v", legacyJSON, err)
	}
	invalid := [][]byte{
		[]byte("sing-box version v1.13.19\n"),
		[]byte("warning\nsing-box version 1.13.19\n"),
		[]byte("sing-box 1.13.19\n"),
		[]byte("sing-box version 1.13.19\nTags: with_quic,,with_utls\n"),
		[]byte("sing-box version 1.13.19\nTags: with_quic;unexpected\n"),
		[]byte("sing-box version 1.13.19\nTags: with_quic\nTags: with_utls\n"),
		[]byte("sing-box version 1.13.19\nCGO: perhaps\n"),
		[]byte("sing-box version 1.13.19\nEnvironment: linux\x00/amd64\n"),
		{0xff, 0xfe, 0xfd},
		[]byte("sing-box version 1.13.19\n" + strings.Repeat("\n", maximumVersionBannerLines+1)),
		[]byte("sing-box version 1.13.19\nTags: " + strings.Repeat("a", maximumFeatureLength+1) + "\n"),
		[]byte(""),
	}
	for _, output := range invalid {
		if _, err := parseVersionOutput(output); err == nil {
			t.Fatalf("parseVersionOutput(%q) succeeded", output)
		}
	}
	oversized := []byte(strings.Repeat("x", maximumVersionBannerBytes+1))
	if _, err := parseVersionOutput(oversized); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("parseVersionOutput(oversized) error = %v, want ErrTooLarge", err)
	}
}

type memoryDownloader struct {
	data  []byte
	calls int
}

type downloaderFunc func(context.Context, string, io.Writer, int64) (int64, error)

func (download downloaderFunc) Download(ctx context.Context, rawURL string, destination io.Writer, maximum int64) (int64, error) {
	return download(ctx, rawURL, destination, maximum)
}

func (downloader *memoryDownloader) Download(_ context.Context, _ string, destination io.Writer, _ int64) (int64, error) {
	downloader.calls++
	written, err := destination.Write(downloader.data)
	return int64(written), err
}

type fakeInspector struct {
	report VersionReport
	err    error
	calls  int
}

type inspectorFunc func(context.Context, string, int64) (VersionReport, error)

func (inspector inspectorFunc) Inspect(ctx context.Context, path string, maximumOutput int64) (VersionReport, error) {
	return inspector(ctx, path, maximumOutput)
}

func (inspector *fakeInspector) Inspect(_ context.Context, _ string, _ int64) (VersionReport, error) {
	inspector.calls++
	return inspector.report, inspector.err
}

func newTestStore(t *testing.T, downloader Downloader, inspector VersionInspector, limits Limits) *Store {
	t.Helper()
	store, err := New(Options{
		Root: filepath.Join(t.TempDir(), "artifacts"), Downloader: downloader, Inspector: inspector, Limits: limits,
	})
	if err != nil {
		t.Fatalf("New Store: %v", err)
	}
	return store
}

func officialAsset(version coreartifact.ExactVersion, digest coreartifact.SHA256, size int64) catalog.Asset {
	name := "sing-box-" + version.String() + "-linux-amd64.tar.gz"
	asset := catalog.Asset{
		RepositoryID: catalog.OfficialRepositoryID, ReleaseID: 11, AssetID: 13,
		Name:        name,
		DownloadURL: "https://github.com/SagerNet/sing-box/releases/download/v" + version.String() + "/" + name,
		Size:        size, Version: version, OperatingSystem: coreartifact.OperatingSystemLinux,
		Architecture: coreartifact.ArchitectureAMD64, Variant: coreartifact.VariantPlain,
	}
	if !digest.IsZero() {
		asset.APIDigest, asset.HasAPIDigest = digest, true
	}
	return asset
}

type tarEntry struct {
	name string
	data []byte
	kind byte
	link string
}

func makeArchive(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: entry.kind, Mode: 0o755, Size: int64(len(entry.data)), Linkname: entry.link}
		if entry.kind == tar.TypeDir || entry.kind == tar.TypeSymlink || entry.kind == tar.TypeLink {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q): %v", entry.name, err)
		}
		if len(entry.data) > 0 {
			if _, err := tarWriter.Write(entry.data); err != nil {
				t.Fatalf("Write(%q): %v", entry.name, err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return compressed.Bytes()
}

func minimalELF(architecture coreartifact.Architecture) []byte {
	data := make([]byte, 64)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2 // ELFCLASS64
	data[5] = 1 // little endian
	data[6] = 1 // ELF version
	data[7] = 0 // System V ABI; Linux static binaries commonly use it.
	binary.LittleEndian.PutUint16(data[16:18], 2)
	machine := uint16(62)
	if architecture == coreartifact.ArchitectureARM64 {
		machine = 183
	}
	binary.LittleEndian.PutUint16(data[18:20], machine)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint16(data[52:54], 64)
	binary.LittleEndian.PutUint16(data[54:56], 56)
	binary.LittleEndian.PutUint16(data[58:60], 64)
	return data
}

func bytesDigest(data []byte) coreartifact.SHA256 {
	return coreartifact.NewSHA256(sha256.Sum256(data))
}

func artifactVersion(t *testing.T, value string) coreartifact.ExactVersion {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(value)
	if err != nil {
		t.Fatalf("ParseExactVersion: %v", err)
	}
	return version
}

func TestFailureTextDoesNotExposeLocalPathOrInspectorOutput(t *testing.T) {
	t.Parallel()
	secret := "sensitive-local-path-and-output"
	version := artifactVersion(t, "1.13.19")
	archive := makeArchive(t, tarEntry{name: "bundle/sing-box", data: minimalELF(coreartifact.ArchitectureAMD64), kind: tar.TypeReg})
	store := newTestStore(t, &memoryDownloader{data: archive}, &fakeInspector{err: errors.New(secret)}, Limits{})
	_, err := store.InstallOfficial(context.Background(), officialAsset(version, bytesDigest(archive), int64(len(archive))))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("safe failure = %v", err)
	}
}

func TestExecVersionInspectorRunsBinaryAndParsesExactVersion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executable shell fixture is Unix-only")
	}
	binaryPath := filepath.Join(t.TempDir(), "sing-box")
	fixture := []byte("#!/bin/sh\n[ \"$1\" = version ] || exit 2\nprintf 'sing-box version 1.13.19\\n\\nTags: with_utls,with_quic\\nCGO: disabled\\n'\n")
	if err := os.WriteFile(binaryPath, fixture, 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	report, err := (ExecVersionInspector{}).Inspect(context.Background(), binaryPath, 1024)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if report.Version.String() != "1.13.19" {
		t.Fatalf("reported version = %q, want 1.13.19", report.Version)
	}
	fingerprintJSON, err := report.FeatureFingerprint.CanonicalJSON()
	if err != nil || string(fingerprintJSON) != `{"status":"reported","features":["with_quic","with_utls"]}` {
		t.Fatalf("reported feature fingerprint = %s, err=%v", fingerprintJSON, err)
	}
}
