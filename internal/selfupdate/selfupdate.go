// SPDX-License-Identifier: GPL-3.0-or-later

// Package selfupdate downloads, verifies, and atomically installs published
// sing-box-panel release binaries.
package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rehuony/sing-box-panel/internal/releasesignature"
	"github.com/rehuony/sing-box-panel/internal/releaseversion"
	"golang.org/x/mod/semver"
)

const (
	checksumAssetName  = "SHA256SUMS"
	signatureAssetName = "SHA256SUMS.sig"
	maxReleaseBytes    = 1 << 20
	maxChecksumBytes   = 1 << 20
	maxSignatureBytes  = 1 << 10
	maxBinaryBytes     = 256 << 20
)

var (
	// defaultLatestReleaseURL can be replaced only at link time for an isolated
	// release smoke test. Production builds use the repository's latest release.
	defaultLatestReleaseURL = "https://api.github.com/repos/rehuony/sing-box-panel/releases/latest"

	// embeddedPublicKey is populated by the formal release build through -X. A
	// release binary without a valid key fails closed before fetching assets.
	embeddedPublicKey string
)

var (
	ErrUnsupportedPlatform    = errors.New("self-update is unsupported on this platform")
	ErrInvalidVersion         = errors.New("self-update requires a release build version")
	ErrVerificationKeyInvalid = errors.New("embedded update verification key is invalid")
	ErrReleaseUnavailable     = errors.New("release is unavailable")
	ErrReleaseInvalid         = errors.New("release metadata is invalid")
	ErrAssetMissing           = errors.New("required release asset is missing")
	ErrSignatureInvalid       = errors.New("release signature is invalid")
	ErrChecksumInvalid        = errors.New("release checksum is invalid")
	ErrExecutableInvalid      = errors.New("current executable is invalid")
)

type Options struct {
	HTTPClient       *http.Client
	LatestReleaseURL string
	GOOS             string
	GOARCH           string
	Executable       func() (string, error)
	PublicKey        ed25519.PublicKey
}

type Updater struct {
	client           *http.Client
	latestReleaseURL string
	goos             string
	goarch           string
	executable       func() (string, error)
	publicKey        ed25519.PublicKey
	publicKeyErr     error
}

type Result struct {
	PreviousVersion string `json:"previous_version"`
	Version         string `json:"version"`
	Updated         bool   `json:"updated"`
	ExecutablePath  string `json:"executable_path,omitempty"`
}

type release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func New(options Options) *Updater {
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	if options.LatestReleaseURL == "" {
		options.LatestReleaseURL = defaultLatestReleaseURL
	}
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.GOARCH == "" {
		options.GOARCH = runtime.GOARCH
	}
	if options.Executable == nil {
		options.Executable = os.Executable
	}
	publicKey := ed25519.PublicKey(bytes.Clone(options.PublicKey))
	var publicKeyErr error
	if len(publicKey) == 0 && embeddedPublicKey != "" {
		publicKey, publicKeyErr = releasesignature.ParsePublicKey(embeddedPublicKey)
	}
	return &Updater{
		client: options.HTTPClient, latestReleaseURL: options.LatestReleaseURL,
		goos: options.GOOS, goarch: options.GOARCH, executable: options.Executable,
		publicKey: publicKey, publicKeyErr: publicKeyErr,
	}
}

func (updater *Updater) Update(ctx context.Context, currentVersion string) (Result, error) {
	if updater.goos != "linux" || updater.goarch != "amd64" && updater.goarch != "arm64" {
		return Result{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, updater.goos, updater.goarch)
	}
	if err := releaseversion.Validate(currentVersion); err != nil {
		return Result{}, fmt.Errorf("%w: got %q", ErrInvalidVersion, currentVersion)
	}
	if updater.publicKeyErr != nil || len(updater.publicKey) != ed25519.PublicKeySize {
		return Result{}, ErrVerificationKeyInvalid
	}

	latest, err := updater.latest(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{PreviousVersion: currentVersion, Version: currentVersion}
	if semver.Compare(currentVersion, latest.TagName) >= 0 {
		return result, nil
	}

	binaryName := "sing-box-panel-" + updater.goos + "-" + updater.goarch
	binaryAsset, err := findAsset(latest.Assets, binaryName)
	if err != nil {
		return Result{}, err
	}
	checksumAsset, err := findAsset(latest.Assets, checksumAssetName)
	if err != nil {
		return Result{}, err
	}
	signatureAsset, err := findAsset(latest.Assets, signatureAssetName)
	if err != nil {
		return Result{}, err
	}
	checksums, err := updater.downloadBytes(ctx, checksumAsset, maxChecksumBytes)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", checksumAssetName, err)
	}
	signature, err := updater.downloadBytes(ctx, signatureAsset, maxSignatureBytes)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", signatureAssetName, err)
	}
	if err := releasesignature.Verify(updater.publicKey, latest.TagName, checksums, signature); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	expected, err := parseChecksum(checksums, binaryName)
	if err != nil {
		return Result{}, err
	}

	targetPath, targetMode, err := updater.executableTarget()
	if err != nil {
		return Result{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(targetPath), ".sing-box-panel-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("stage update beside executable: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()

	actual, err := updater.downloadFile(ctx, binaryAsset, temporary, maxBinaryBytes)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", binaryName, err)
	}
	if actual != expected {
		return Result{}, fmt.Errorf(
			"%w: %s expected %x, got %x",
			ErrChecksumInvalid, binaryName, expected, actual,
		)
	}
	if err := temporary.Chmod(targetMode); err != nil {
		return Result{}, fmt.Errorf("set staged executable permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return Result{}, fmt.Errorf("sync staged executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Result{}, fmt.Errorf("close staged executable: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return Result{}, fmt.Errorf("replace current executable: %w", err)
	}
	if err := syncDirectory(filepath.Dir(targetPath)); err != nil {
		return Result{}, err
	}

	result.Updated = true
	result.Version = latest.TagName
	result.ExecutablePath = targetPath
	return result, nil
}
