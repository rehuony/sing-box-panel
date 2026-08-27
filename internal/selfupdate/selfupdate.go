// SPDX-License-Identifier: GPL-3.0-or-later

// Package selfupdate downloads, verifies, and atomically installs published
// sing-box-panel release binaries.
package selfupdate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/releasegate"
	"github.com/rehuony/sing-box-panel/internal/releasesignature"
	"golang.org/x/mod/semver"
)

const (
	defaultLatestReleaseURL = "https://api.github.com/repos/rehuony/sing-box-panel/releases/latest"
	checksumAssetName       = "SHA256SUMS"
	signatureAssetName      = "SHA256SUMS.sig"
	maxReleaseBytes         = 1 << 20
	maxChecksumBytes        = 1 << 20
	maxSignatureBytes       = 1 << 10
	maxBinaryBytes          = 256 << 20
)

// embeddedPublicKey is populated by the formal release build through -X. A
// release binary without a valid key fails closed before fetching assets.
var embeddedPublicKey string

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
	if err := releasegate.ValidateReleaseVersion(currentVersion); err != nil {
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

func (updater *Updater) latest(ctx context.Context) (release, error) {
	request, err := updater.request(ctx, updater.latestReleaseURL, "application/vnd.github+json")
	if err != nil {
		return release{}, err
	}
	response, err := updater.client.Do(request)
	if err != nil {
		return release{}, fmt.Errorf("%w: query latest release: %w", ErrReleaseUnavailable, err)
	}
	defer response.Body.Close()
	if err := updater.validateResponseURL(response); err != nil {
		return release{}, err
	}
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("%w: GitHub returned %s", ErrReleaseUnavailable, response.Status)
	}
	if response.ContentLength > maxReleaseBytes {
		return release{}, fmt.Errorf("%w: metadata exceeds %d bytes", ErrReleaseInvalid, maxReleaseBytes)
	}
	data, err := readBounded(response.Body, maxReleaseBytes)
	if err != nil {
		return release{}, fmt.Errorf("%w: read metadata: %w", ErrReleaseInvalid, err)
	}
	var latest release
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&latest); err != nil {
		return release{}, fmt.Errorf("%w: decode metadata: %v", ErrReleaseInvalid, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return release{}, fmt.Errorf("%w: metadata contains trailing content", ErrReleaseInvalid)
	}
	if latest.Draft || latest.Prerelease {
		return release{}, fmt.Errorf("%w: latest endpoint returned a draft or prerelease", ErrReleaseInvalid)
	}
	if err := releasegate.ValidateReleaseVersion(latest.TagName); err != nil {
		return release{}, fmt.Errorf("%w: tag %q is not strict SemVer", ErrReleaseInvalid, latest.TagName)
	}
	return latest, nil
}

func (updater *Updater) downloadBytes(ctx context.Context, value asset, maximum int64) ([]byte, error) {
	request, err := updater.request(ctx, value.BrowserDownloadURL, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	response, err := updater.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReleaseUnavailable, err)
	}
	defer response.Body.Close()
	if err := updater.validateResponseURL(response); err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: asset returned %s", ErrReleaseUnavailable, response.Status)
	}
	if value.Size > maximum || response.ContentLength > maximum {
		return nil, fmt.Errorf("%w: asset exceeds %d bytes", ErrReleaseInvalid, maximum)
	}
	data, err := readBounded(response.Body, maximum)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReleaseInvalid, err)
	}
	return data, nil
}

func (updater *Updater) downloadFile(ctx context.Context, value asset, destination io.Writer, maximum int64) ([sha256.Size]byte, error) {
	request, err := updater.request(ctx, value.BrowserDownloadURL, "application/octet-stream")
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	response, err := updater.client.Do(request)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: %w", ErrReleaseUnavailable, err)
	}
	defer response.Body.Close()
	if err := updater.validateResponseURL(response); err != nil {
		return [sha256.Size]byte{}, err
	}
	if response.StatusCode != http.StatusOK {
		return [sha256.Size]byte{}, fmt.Errorf("%w: asset returned %s", ErrReleaseUnavailable, response.Status)
	}
	if value.Size > maximum || response.ContentLength > maximum {
		return [sha256.Size]byte{}, fmt.Errorf("%w: asset exceeds %d bytes", ErrReleaseInvalid, maximum)
	}

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: read asset: %w", ErrReleaseUnavailable, err)
	}
	if written > maximum {
		return [sha256.Size]byte{}, fmt.Errorf("%w: asset exceeds %d bytes", ErrReleaseInvalid, maximum)
	}
	if written == 0 {
		return [sha256.Size]byte{}, fmt.Errorf("%w: binary asset is empty", ErrReleaseInvalid)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func (updater *Updater) request(ctx context.Context, rawURL, accept string) (*http.Request, error) {
	parsed, err := updater.validateURL(rawURL)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "sing-box-panel-self-update")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return request, nil
}

func (updater *Updater) validateResponseURL(response *http.Response) error {
	if response == nil || response.Request == nil || response.Request.URL == nil {
		return fmt.Errorf("%w: response has no final URL", ErrReleaseInvalid)
	}
	_, err := updater.validateURL(response.Request.URL.String())
	return err
}

func (updater *Updater) validateURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("%w: invalid release URL %q", ErrReleaseInvalid, rawURL)
	}
	if parsed.Scheme == "http" {
		base, baseErr := url.Parse(updater.latestReleaseURL)
		if baseErr != nil || base.Scheme != "http" || base.Host != parsed.Host {
			return nil, fmt.Errorf("%w: insecure release URL %q", ErrReleaseInvalid, rawURL)
		}
	}
	return parsed, nil
}

func (updater *Updater) executableTarget() (string, os.FileMode, error) {
	rawPath, err := updater.executable()
	if err != nil {
		return "", 0, fmt.Errorf("%w: resolve path: %w", ErrExecutableInvalid, err)
	}
	absolute, err := filepath.Abs(rawPath)
	if err != nil {
		return "", 0, fmt.Errorf("%w: resolve absolute path: %w", ErrExecutableInvalid, err)
	}
	path, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", 0, fmt.Errorf("%w: resolve symlinks: %w", ErrExecutableInvalid, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, fmt.Errorf("%w: inspect path: %w", ErrExecutableInvalid, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", 0, fmt.Errorf("%w: path must be a regular executable file", ErrExecutableInvalid)
	}
	return path, info.Mode().Perm(), nil
}

func findAsset(assets []asset, name string) (asset, error) {
	var found asset
	matches := 0
	for _, candidate := range assets {
		if candidate.Name == name {
			found = candidate
			matches++
		}
	}
	if matches == 0 {
		return asset{}, fmt.Errorf("%w: %s", ErrAssetMissing, name)
	}
	if matches != 1 || found.BrowserDownloadURL == "" || found.Size < 0 {
		return asset{}, fmt.Errorf("%w: asset %s is ambiguous or malformed", ErrReleaseInvalid, name)
	}
	return found, nil
}

func parseChecksum(data []byte, assetName string) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte
	found := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return result, fmt.Errorf("%w: malformed %s entry", ErrChecksumInvalid, checksumAssetName)
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if name != assetName {
			continue
		}
		if found {
			return result, fmt.Errorf("%w: duplicate entry for %s", ErrChecksumInvalid, assetName)
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return result, fmt.Errorf("%w: malformed digest for %s", ErrChecksumInvalid, assetName)
		}
		copy(result[:], digest)
		found = true
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("%w: read %s: %v", ErrChecksumInvalid, checksumAssetName, err)
	}
	if !found {
		return result, fmt.Errorf("%w: no entry for %s", ErrChecksumInvalid, assetName)
	}
	return result, nil
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("response exceeds %d bytes", maximum)
	}
	return data, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open executable directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync executable directory: %w", err)
	}
	return nil
}
