// SPDX-License-Identifier: GPL-3.0-or-later

// Package selfupdate fetches, verifies and installs signed release binaries
// of the panel and the node. Its steps are shared by the panel's update
// command, the panel's node release cache, both services' updates from the
// panel and the install step of a system service's unit: fetch a release's
// files, verify staged files, and install a verified binary. Updates go one
// way: an installed release replaces the binary, which is not kept.
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

	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
	"golang.org/x/mod/semver"
)

const (
	// ChecksumsName and SignatureName are the signed checksum manifest of a
	// release and its detached signature.
	ChecksumsName = "SHA256SUMS"
	SignatureName = "SHA256SUMS.sig"

	maxReleaseBytes = 1 << 20
	// MaxChecksumBytes, MaxSignatureBytes and MaxBinaryBytes bound the files
	// of a release.
	MaxChecksumBytes  = 1 << 20
	MaxSignatureBytes = 1 << 10
	MaxBinaryBytes    = 256 << 20
)

var (
	// defaultReleasesURL can be replaced only at link time for an isolated
	// release smoke test. Production builds use the repository's releases:
	// the latest stable release is defaultReleasesURL/latest and one exact
	// release is defaultReleasesURL/tags/VERSION.
	defaultReleasesURL = "https://api.github.com/repos/rehuony/sing-box-panel/releases"

	// embeddedPublicKey is populated by the formal release build through -X. A
	// release binary without a valid key fails closed before fetching assets.
	embeddedPublicKey string
)

var (
	ErrUnsupportedPlatform     = errors.New("self-update is unsupported on this platform")
	ErrInvalidVersion          = errors.New("self-update requires a release build version")
	ErrVerificationKeyInvalid  = errors.New("embedded update verification key is invalid")
	ErrReleaseUnavailable      = errors.New("release is unavailable")
	ErrReleaseInvalid          = errors.New("release metadata is invalid")
	ErrAssetMissing            = errors.New("required release asset is missing")
	ErrSignatureInvalid        = errors.New("release signature is invalid")
	ErrChecksumInvalid         = errors.New("release checksum is invalid")
	ErrExecutableInvalid       = errors.New("current executable is invalid")
	ErrExecutableChanged       = errors.New("current executable changed during self-update")
	ErrStagedExecutableInvalid = errors.New("staged executable identity is invalid")
	ErrStagedFileInvalid       = errors.New("staged release file is missing, not a regular file or too large")
	// ErrUpToDate reports a release of the version that already runs.
	ErrUpToDate = errors.New("the release is the version that already runs")
	// ErrVersionNotNewer refuses a release older than the version that runs.
	ErrVersionNotNewer = errors.New("the release is older than the version that runs")
	// ErrExecutableNotWritable reports a process that may not replace its
	// executable and has no unit to install a staged release for it.
	ErrExecutableNotWritable = errors.New("this process may not replace its executable; run the update command with write access to it")
)

// Binary is one of the release's executables. It selects the release asset
// and the Go command path a staged binary must carry.
type Binary string

const (
	Panel Binary = "sing-box-panel"
	Node  Binary = "sing-box-node"
)

const modulePath = "github.com/rehuony/sing-box-panel"

// AssetName is the release asset of the binary for a platform.
func (binary Binary) AssetName(goos, goarch string) string {
	return string(binary) + "-" + goos + "-" + goarch
}

func (binary Binary) commandPath() string { return modulePath + "/cmd/" + string(binary) }

func (binary Binary) valid() bool { return binary == Panel || binary == Node }

// SupportedPlatform reports whether releases carry binaries for a platform.
func SupportedPlatform(goos, goarch string) bool {
	return goos == "linux" && (goarch == "amd64" || goarch == "arm64")
}

type Options struct {
	// Binary is the executable this updater fetches, verifies and installs.
	Binary     Binary
	HTTPClient *http.Client
	// ReleasesURL is the GitHub releases API of the repository; tests point
	// it at a local server.
	ReleasesURL string
	// Token, if set, authenticates GitHub API requests.
	Token      string
	GOOS       string
	GOARCH     string
	Executable func() (string, error)
	// PublicKey replaces the release key embedded at build time.
	PublicKey ed25519.PublicKey
}

type Updater struct {
	binary                   Binary
	client                   *http.Client
	releasesURL              string
	token                    string
	goos                     string
	goarch                   string
	executable               func() (string, error)
	validateStagedExecutable func(path string, binary Binary, goos, goarch string) error
	publicKey                ed25519.PublicKey
	publicKeyErr             error
	afterTargetSnapshot      func()
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
	if options.ReleasesURL == "" {
		options.ReleasesURL = defaultReleasesURL
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
		publicKey, publicKeyErr = releasecontract.ParsePublicKey(embeddedPublicKey)
	}
	return &Updater{
		binary: options.Binary, client: options.HTTPClient, releasesURL: options.ReleasesURL, token: options.Token,
		goos: options.GOOS, goarch: options.GOARCH, executable: options.Executable,
		validateStagedExecutable: validateStagedExecutable,
		publicKey:                publicKey, publicKeyErr: publicKeyErr,
	}
}

// AssetName is the release asset this updater installs.
func (updater *Updater) AssetName() string {
	return updater.binary.AssetName(updater.goos, updater.goarch)
}

// ready checks what every update needs before it touches the network or
// the executable: a known binary, a supported platform, a release build and
// a valid verification key.
func (updater *Updater) ready(currentVersion string) error {
	if !updater.binary.valid() {
		return fmt.Errorf("%w: unknown binary %q", ErrUnsupportedPlatform, updater.binary)
	}
	if !SupportedPlatform(updater.goos, updater.goarch) {
		return fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, updater.goos, updater.goarch)
	}
	if err := releasecontract.ValidateVersion(currentVersion); err != nil {
		return fmt.Errorf("%w: got %q", ErrInvalidVersion, currentVersion)
	}
	return updater.keyReady()
}

func (updater *Updater) keyReady() error {
	if updater.publicKeyErr != nil || len(updater.publicKey) != ed25519.PublicKeySize {
		return ErrVerificationKeyInvalid
	}
	return nil
}

// Ready reports whether the process can update from currentVersion at all.
func (updater *Updater) Ready(currentVersion string) error { return updater.ready(currentVersion) }

// Latest returns the version of the newest stable release.
func (updater *Updater) Latest(ctx context.Context) (string, error) {
	latest, err := updater.release(ctx, "")
	if err != nil {
		return "", err
	}
	return latest.TagName, nil
}

// Update installs the latest stable release over the current executable
// when it is newer than currentVersion. The checksum manifest's signature is
// verified before the executable is inspected or the binary downloaded, and
// the binary is staged beside the executable, verified and installed under
// the update lock.
func (updater *Updater) Update(ctx context.Context, currentVersion string, report ProgressFunc) (Result, error) {
	if err := updater.ready(currentVersion); err != nil {
		return Result{}, err
	}

	report.emit(Progress{Stage: StageCheckRelease})
	latest, err := updater.release(ctx, "")
	if err != nil {
		return Result{}, err
	}
	result := Result{PreviousVersion: currentVersion, Version: currentVersion}
	if semver.Compare(currentVersion, latest.TagName) >= 0 {
		return result, nil
	}
	binaryAsset, err := findAsset(latest.Assets, updater.AssetName())
	if err != nil {
		return Result{}, err
	}
	report.emit(Progress{Stage: StageVerifyRelease, Version: latest.TagName})
	signed, err := updater.manifest(ctx, latest)
	if err != nil {
		return Result{}, err
	}
	expected, err := parseChecksum(signed.checksums, updater.AssetName())
	if err != nil {
		return Result{}, err
	}

	report.emit(Progress{Stage: StagePrepare})
	targetPath, targetMode, err := updater.executableTarget()
	if err != nil {
		return Result{}, err
	}
	targetDigest, err := executableDigest(targetPath)
	if err != nil {
		return Result{}, err
	}
	if updater.afterTargetSnapshot != nil {
		updater.afterTargetSnapshot()
	}
	lock, err := acquireUpdateLock(ctx, targetPath)
	if err != nil {
		return Result{}, fmt.Errorf("acquire update lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if err := requireExecutableDigest(targetPath, targetDigest); err != nil {
		return Result{}, err
	}
	confirmedLatest, err := updater.release(ctx, "")
	if err != nil {
		return Result{}, err
	}
	if confirmedLatest.TagName != latest.TagName {
		return Result{}, fmt.Errorf(
			"%w: latest release changed from %s to %s; retry",
			ErrReleaseInvalid, latest.TagName, confirmedLatest.TagName,
		)
	}

	stage, err := os.MkdirTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+"-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("stage update beside executable: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := signed.write(stage); err != nil {
		return Result{}, err
	}
	if err := updater.downloadAsset(ctx, binaryAsset, stage, expected, report); err != nil {
		return Result{}, err
	}
	report.emit(Progress{Stage: StageVerifyBinary})
	staged, err := updater.Verify(stage, latest.TagName, currentVersion)
	if err != nil {
		return Result{}, err
	}
	report.emit(Progress{Stage: StageInstall})
	if err := updater.installLocked(ctx, staged, targetPath, targetMode, targetDigest); err != nil {
		return Result{}, err
	}
	result.Updated = true
	result.Version = latest.TagName
	result.ExecutablePath = targetPath
	return result, nil
}
