// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/releasesignature"
)

func TestUpdateDownloadsVerifiesAndAtomicallyReplacesExecutable(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "sing-box-panel")
	if err := os.WriteFile(target, []byte("old binary"), 0o750); err != nil {
		t.Fatal(err)
	}
	newBinary := []byte("new verified binary")
	digest := sha256.Sum256(newBinary)
	checksums := fmt.Sprintf("%x  sing-box-panel-linux-amd64\n", digest)

	server, publicKey := signedReleaseServer(t, "v1.3.0", map[string][]byte{
		"sing-box-panel-linux-amd64": newBinary,
		checksumAssetName:            []byte(checksums),
	}, nil)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: publicKey,
	})
	updater.validateStagedExecutable = func(string, string, string) error { return nil }

	result, err := updater.Update(context.Background(), "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.PreviousVersion != "v1.2.3" || result.Version != "v1.3.0" || result.ExecutablePath != resolvedTarget {
		t.Fatalf("result = %+v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(newBinary) {
		t.Fatalf("executable = %q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("executable mode = %o", info.Mode().Perm())
	}
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".sing-box-panel-update-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 0 {
		t.Fatalf("staged files remain: %v", staged)
	}
}

func TestUpdateRejectsStagedFileWithoutPanelBuildIdentity(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "sing-box-panel")
	original := []byte("old binary")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	newBinary := []byte("signed but not a sing-box-panel Go binary")
	digest := sha256.Sum256(newBinary)
	server, publicKey := signedReleaseServer(t, "v1.3.0", map[string][]byte{
		"sing-box-panel-linux-amd64": newBinary,
		checksumAssetName: []byte(fmt.Sprintf(
			"%x  sing-box-panel-linux-amd64\n", digest,
		)),
	}, nil)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: publicKey,
	})

	_, err := updater.Update(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrStagedExecutableInvalid) {
		t.Fatalf("error = %v, want ErrStagedExecutableInvalid", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("executable changed to %q", data)
	}
}

func TestUpdateHonorsCancellationAtAtomicCommitBoundary(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "sing-box-panel")
	original := []byte("old binary")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	newBinary := []byte("new verified binary")
	digest := sha256.Sum256(newBinary)
	server, publicKey := signedReleaseServer(t, "v1.3.0", map[string][]byte{
		"sing-box-panel-linux-amd64": newBinary,
		checksumAssetName: []byte(fmt.Sprintf(
			"%x  sing-box-panel-linux-amd64\n", digest,
		)),
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: publicKey,
	})
	updater.validateStagedExecutable = func(string, string, string) error {
		cancel()
		return nil
	}

	_, err := updater.Update(ctx, "v1.2.3")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("executable changed to %q", data)
	}
}

func TestUpdateRejectsExecutableChangedWhileWaitingForLock(t *testing.T) {
	target := filepath.Join(t.TempDir(), "sing-box-panel")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireUpdateLock(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if lock != nil {
			_ = lock.Close()
		}
	}()

	newBinary := []byte("new verified binary")
	digest := sha256.Sum256(newBinary)
	server, publicKey := signedReleaseServer(t, "v1.3.0", map[string][]byte{
		"sing-box-panel-linux-amd64": newBinary,
		checksumAssetName: []byte(fmt.Sprintf(
			"%x  sing-box-panel-linux-amd64\n", digest,
		)),
	}, nil)
	snapshotTaken := make(chan struct{})
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: publicKey,
	})
	updater.afterTargetSnapshot = func() { close(snapshotTaken) }
	result := make(chan error, 1)
	go func() {
		_, updateErr := updater.Update(context.Background(), "v1.2.3")
		result <- updateErr
	}()

	<-snapshotTaken
	changed := []byte("replacement from concurrent updater")
	if err := os.WriteFile(target, changed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	lock = nil
	if err := <-result; !errors.Is(err, ErrExecutableChanged) {
		t.Fatalf("error = %v, want ErrExecutableChanged", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(changed) {
		t.Fatalf("executable changed to %q", data)
	}
}

func TestUpdateRejectsLatestReleaseChangingInsideLock(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "sing-box-panel")
	original := []byte("old binary")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte("new verified binary")
	digest := sha256.Sum256(binary)
	checksums := []byte(fmt.Sprintf("%x  sing-box-panel-linux-amd64\n", digest))
	signature, err := releasesignature.Sign(privateKey, "v1.3.0", checksums)
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{
		"sing-box-panel-linux-amd64": binary,
		checksumAssetName:            checksums,
		signatureAssetName:           signature,
	}
	var latestRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/latest" {
			tag := "v1.3.0"
			if latestRequests.Add(1) > 1 {
				tag = "v1.4.0"
			}
			values := make([]asset, 0, len(assets))
			for name, data := range assets {
				values = append(values, asset{
					Name: name, BrowserDownloadURL: server.URL + "/asset/" + name, Size: int64(len(data)),
				})
			}
			writer.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(writer).Encode(release{TagName: tag, Assets: values}); err != nil {
				t.Errorf("encode release: %v", err)
			}
			return
		}
		if strings.HasPrefix(request.URL.Path, "/asset/") {
			data, found := assets[strings.TrimPrefix(request.URL.Path, "/asset/")]
			if !found {
				http.NotFound(writer, request)
				return
			}
			_, _ = writer.Write(data)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(server.Close)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: publicKey,
	})

	_, err = updater.Update(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrReleaseInvalid) {
		t.Fatalf("error = %v, want ErrReleaseInvalid", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("executable changed to %q", data)
	}
}

func TestValidateStagedBuildIdentity(t *testing.T) {
	t.Parallel()

	valid := &debug.BuildInfo{
		Path: panelCommandPath,
		Main: debug.Module{Path: panelModulePath},
		Settings: []debug.BuildSetting{
			{Key: "CGO_ENABLED", Value: "0"},
			{Key: "GOARCH", Value: "amd64"},
			{Key: "GOOS", Value: "linux"},
		},
	}
	if err := validateStagedBuildIdentity(valid, "linux", "amd64"); err != nil {
		t.Fatalf("valid identity: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*debug.BuildInfo)
	}{
		{name: "command", mutate: func(value *debug.BuildInfo) { value.Path = panelModulePath + "/cmd/other" }},
		{name: "module", mutate: func(value *debug.BuildInfo) { value.Main.Path = "example.com/other" }},
		{name: "architecture", mutate: func(value *debug.BuildInfo) { value.Settings[1].Value = "arm64" }},
		{name: "operating system", mutate: func(value *debug.BuildInfo) { value.Settings[2].Value = "darwin" }},
		{name: "cgo", mutate: func(value *debug.BuildInfo) { value.Settings[0].Value = "1" }},
		{name: "missing setting", mutate: func(value *debug.BuildInfo) { value.Settings = value.Settings[:2] }},
		{name: "duplicate setting", mutate: func(value *debug.BuildInfo) {
			value.Settings = append(value.Settings, debug.BuildSetting{Key: "GOOS", Value: "linux"})
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := *valid
			candidate.Main = valid.Main
			candidate.Settings = append([]debug.BuildSetting(nil), valid.Settings...)
			testCase.mutate(&candidate)
			if err := validateStagedBuildIdentity(&candidate, "linux", "amd64"); !errors.Is(err, ErrStagedExecutableInvalid) {
				t.Fatalf("error = %v, want ErrStagedExecutableInvalid", err)
			}
		})
	}
}

func TestUpdateChecksumFailureLeavesExecutableUntouched(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "sing-box-panel")
	original := []byte("old binary")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	wrongDigest := sha256.Sum256([]byte("different binary"))
	server, publicKey := signedReleaseServer(t, "v1.3.0", map[string][]byte{
		"sing-box-panel-linux-amd64": []byte("unverified binary"),
		checksumAssetName: []byte(fmt.Sprintf(
			"%x  sing-box-panel-linux-amd64\n", wrongDigest,
		)),
	}, nil)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: publicKey,
	})

	_, err := updater.Update(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrChecksumInvalid) {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("executable changed to %q", data)
	}
}

func TestUpdateDoesNotDownloadAssetsOrDowngrade(t *testing.T) {
	t.Parallel()

	var assetRequests atomic.Int32
	server, publicKey := signedReleaseServer(t, "v1.2.2", map[string][]byte{
		"sing-box-panel-linux-amd64": []byte("binary"),
		checksumAssetName:            []byte("checksum"),
	}, &assetRequests)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", PublicKey: publicKey,
		Executable: func() (string, error) {
			t.Fatal("up-to-date check must not inspect or mutate the executable")
			return "", nil
		},
	})

	result, err := updater.Update(context.Background(), "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.PreviousVersion != "v1.2.3" || result.Version != "v1.2.3" {
		t.Fatalf("result = %+v", result)
	}
	if got := assetRequests.Load(); got != 0 {
		t.Fatalf("asset requests = %d", got)
	}
}

func TestUpdateAcceptsStrictPrereleaseAndBuildMetadata(t *testing.T) {
	t.Parallel()

	const version = "v1.2.3-rc.1+linux.amd64"
	var assetRequests atomic.Int32
	server, publicKey := signedReleaseServer(t, version, map[string][]byte{
		"sing-box-panel-linux-amd64": []byte("binary"),
		checksumAssetName:            []byte("checksum"),
	}, &assetRequests)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", PublicKey: publicKey,
		Executable: func() (string, error) {
			t.Fatal("an up-to-date check must not inspect the executable")
			return "", nil
		},
	})

	result, err := updater.Update(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.PreviousVersion != version || result.Version != version {
		t.Fatalf("result = %+v", result)
	}
	if got := assetRequests.Load(); got != 0 {
		t.Fatalf("asset requests = %d", got)
	}
}

func TestUpdateRejectsUnsupportedAndDevelopmentBuildsBeforeNetwork(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		goarch  string
		version string
		want    error
	}{
		{name: "operating system", goos: "darwin", goarch: "arm64", version: "v1.2.3", want: ErrUnsupportedPlatform},
		{name: "architecture", goos: "linux", goarch: "riscv64", version: "v1.2.3", want: ErrUnsupportedPlatform},
		{name: "development build", goos: "linux", goarch: "amd64", version: "dev", want: ErrInvalidVersion},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			updater := New(Options{
				LatestReleaseURL: "http://127.0.0.1:1/latest",
				GOOS:             testCase.goos, GOARCH: testCase.goarch,
			})
			_, err := updater.Update(context.Background(), testCase.version)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestUpdatePreservesContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	updater := New(Options{
		LatestReleaseURL: "http://127.0.0.1:1/latest",
		GOOS:             "linux", GOARCH: "amd64", PublicKey: newPublicKey(t),
	})
	_, err := updater.Update(ctx, "v1.2.3")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestReleaseURLsRequireHTTPSOutsideInjectedHTTPOrigin(t *testing.T) {
	t.Parallel()

	production := New(Options{})
	if _, err := production.validateURL("http://github.com/release"); !errors.Is(err, ErrReleaseInvalid) {
		t.Fatalf("production HTTP error = %v", err)
	}
	if _, err := production.validateURL("https://github.com/release"); err != nil {
		t.Fatalf("production HTTPS error = %v", err)
	}

	testUpdater := New(Options{LatestReleaseURL: "http://127.0.0.1:1234/latest"})
	if _, err := testUpdater.validateURL("http://127.0.0.1:1234/asset"); err != nil {
		t.Fatalf("same injected HTTP origin error = %v", err)
	}
	if _, err := testUpdater.validateURL("http://127.0.0.1:4321/asset"); !errors.Is(err, ErrReleaseInvalid) {
		t.Fatalf("different HTTP origin error = %v", err)
	}
}

func TestUpdateRejectsIncompleteOrMalformedRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     string
		assets  map[string][]byte
		wantErr error
	}{
		{
			name: "non-strict tag", tag: "v1.3", assets: map[string][]byte{},
			wantErr: ErrReleaseInvalid,
		},
		{
			name: "missing checksum", tag: "v1.3.0",
			assets:  map[string][]byte{"sing-box-panel-linux-amd64": []byte("binary")},
			wantErr: ErrAssetMissing,
		},
		{
			name: "missing binary", tag: "v1.3.0",
			assets:  map[string][]byte{checksumAssetName: []byte("checksum")},
			wantErr: ErrAssetMissing,
		},
		{
			name: "missing signature", tag: "v1.3.0",
			assets: map[string][]byte{
				"sing-box-panel-linux-amd64": []byte("binary"),
				checksumAssetName:            []byte("checksum"),
			},
			wantErr: ErrAssetMissing,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := releaseServer(t, testCase.tag, testCase.assets, nil)
			updater := New(Options{
				HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
				GOOS: "linux", GOARCH: "amd64", PublicKey: newPublicKey(t),
			})
			_, err := updater.Update(context.Background(), "v1.2.3")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestUpdateRejectsUntrustedSignatureBeforeDownloadingBinary(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "sing-box-panel")
	original := []byte("old binary")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	newBinary := []byte("attacker-controlled binary")
	digest := sha256.Sum256(newBinary)
	checksums := []byte(fmt.Sprintf("%x  sing-box-panel-linux-amd64\n", digest))
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := releasesignature.Sign(privateKey, "v1.3.0", checksums)
	if err != nil {
		t.Fatal(err)
	}
	var assetRequests atomic.Int32
	server := releaseServer(t, "v1.3.0", map[string][]byte{
		"sing-box-panel-linux-amd64": newBinary,
		checksumAssetName:            checksums,
		signatureAssetName:           signature,
	}, &assetRequests)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", Executable: func() (string, error) { return target, nil },
		PublicKey: newPublicKey(t),
	})

	_, err = updater.Update(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("error = %v", err)
	}
	if got := assetRequests.Load(); got != 2 {
		t.Fatalf("asset requests = %d, want checksum and signature only", got)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("executable changed to %q", data)
	}
}

func TestUpdateRejectsMissingVerificationKeyBeforeNetwork(t *testing.T) {
	t.Parallel()

	updater := New(Options{
		LatestReleaseURL: "http://127.0.0.1:1/latest",
		GOOS:             "linux", GOARCH: "amd64",
	})
	_, err := updater.Update(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrVerificationKeyInvalid) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateRejectsSignatureReplayedUnderDifferentVersion(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte("previously signed binary")
	digest := sha256.Sum256(binary)
	checksums := []byte(fmt.Sprintf("%x  sing-box-panel-linux-amd64\n", digest))
	signature, err := releasesignature.Sign(privateKey, "v1.3.0", checksums)
	if err != nil {
		t.Fatal(err)
	}
	var assetRequests atomic.Int32
	server := releaseServer(t, "v1.4.0", map[string][]byte{
		"sing-box-panel-linux-amd64": binary,
		checksumAssetName:            checksums,
		signatureAssetName:           signature,
	}, &assetRequests)
	updater := New(Options{
		HTTPClient: server.Client(), LatestReleaseURL: server.URL + "/latest",
		GOOS: "linux", GOARCH: "amd64", PublicKey: publicKey,
		Executable: func() (string, error) {
			t.Fatal("replayed signature must fail before inspecting the executable")
			return "", nil
		},
	})

	_, err = updater.Update(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("error = %v", err)
	}
	if got := assetRequests.Load(); got != 2 {
		t.Fatalf("asset requests = %d, want checksum and signature only", got)
	}
}

func TestParseChecksumRequiresOneExactValidEntry(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("a", sha256.Size*2)
	tests := []struct {
		name string
		data string
		ok   bool
	}{
		{name: "sha256sum", data: digest + "  sing-box-panel-linux-amd64\n", ok: true},
		{name: "binary marker", data: digest + " *sing-box-panel-linux-amd64\n", ok: true},
		{name: "missing", data: digest + "  sing-box-panel-linux-arm64\n"},
		{name: "duplicate", data: digest + "  sing-box-panel-linux-amd64\n" + digest + "  sing-box-panel-linux-amd64\n"},
		{name: "bad digest", data: "not-a-digest  sing-box-panel-linux-amd64\n"},
		{name: "extra field", data: digest + "  sing-box-panel-linux-amd64 extra\n"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseChecksum([]byte(testCase.data), "sing-box-panel-linux-amd64")
			if testCase.ok && err != nil {
				t.Fatal(err)
			}
			if !testCase.ok && !errors.Is(err, ErrChecksumInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestFindAssetRequiresOneWellFormedMatch(t *testing.T) {
	t.Parallel()

	const name = "sing-box-panel-linux-amd64"
	valid := asset{Name: name, BrowserDownloadURL: "https://example.com/binary", Size: 1}
	for _, test := range []struct {
		name    string
		assets  []asset
		wantErr error
	}{
		{name: "valid", assets: []asset{valid}},
		{name: "missing", assets: []asset{{Name: "other"}}, wantErr: ErrAssetMissing},
		{name: "duplicate", assets: []asset{valid, valid}, wantErr: ErrReleaseInvalid},
		{name: "missing URL", assets: []asset{{Name: name, Size: 1}}, wantErr: ErrReleaseInvalid},
		{name: "negative size", assets: []asset{{Name: name, BrowserDownloadURL: valid.BrowserDownloadURL, Size: -1}}, wantErr: ErrReleaseInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := findAsset(test.assets, name)
			if test.wantErr == nil {
				if err != nil {
					t.Fatal(err)
				}
				if got != valid {
					t.Fatalf("findAsset() = %+v, want %+v", got, valid)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("findAsset() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func releaseServer(t *testing.T, tag string, assets map[string][]byte, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/latest" {
			values := make([]asset, 0, len(assets))
			for name, data := range assets {
				values = append(values, asset{
					Name: name, BrowserDownloadURL: server.URL + "/asset/" + name, Size: int64(len(data)),
				})
			}
			writer.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(writer).Encode(release{TagName: tag, Assets: values}); err != nil {
				t.Errorf("encode release: %v", err)
			}
			return
		}
		if strings.HasPrefix(request.URL.Path, "/asset/") {
			if requests != nil {
				requests.Add(1)
			}
			name := strings.TrimPrefix(request.URL.Path, "/asset/")
			data, found := assets[name]
			if !found {
				http.NotFound(writer, request)
				return
			}
			_, _ = writer.Write(data)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(server.Close)
	return server
}

func signedReleaseServer(
	t *testing.T,
	tag string,
	assets map[string][]byte,
	requests *atomic.Int32,
) (*httptest.Server, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signedAssets := make(map[string][]byte, len(assets)+1)
	for name, data := range assets {
		signedAssets[name] = data
	}
	checksums, found := signedAssets[checksumAssetName]
	if found {
		signature, signErr := releasesignature.Sign(privateKey, tag, checksums)
		if signErr != nil {
			t.Fatal(signErr)
		}
		signedAssets[signatureAssetName] = signature
	}
	return releaseServer(t, tag, signedAssets, requests), publicKey
}

func newPublicKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return publicKey
}
