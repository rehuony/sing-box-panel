// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
)

// release reads the latest stable release, or the release of version when
// version is set.
func (updater *Updater) release(ctx context.Context, version string) (release, error) {
	endpoint := strings.TrimSuffix(updater.releasesURL, "/") + "/latest"
	if version != "" {
		if err := releasecontract.ValidateVersion(version); err != nil {
			return release{}, fmt.Errorf("%w: %q is not a release version", ErrReleaseInvalid, version)
		}
		endpoint = strings.TrimSuffix(updater.releasesURL, "/") + "/tags/" + url.PathEscape(version)
	}
	request, err := updater.request(ctx, endpoint, "application/vnd.github+json", true)
	if err != nil {
		return release{}, err
	}
	response, err := updater.client.Do(request)
	if err != nil {
		return release{}, fmt.Errorf("%w: query release: %w", ErrReleaseUnavailable, err)
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
	var found release
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&found); err != nil {
		return release{}, fmt.Errorf("%w: decode metadata: %v", ErrReleaseInvalid, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return release{}, fmt.Errorf("%w: metadata contains trailing content", ErrReleaseInvalid)
	}
	if found.Draft || version == "" && found.Prerelease {
		return release{}, fmt.Errorf("%w: the release endpoint returned a draft or prerelease", ErrReleaseInvalid)
	}
	if err := releasecontract.ValidateVersion(found.TagName); err != nil {
		return release{}, fmt.Errorf("%w: tag %q is not strict SemVer", ErrReleaseInvalid, found.TagName)
	}
	if version != "" && found.TagName != version {
		return release{}, fmt.Errorf("%w: asked for %s, got %s", ErrReleaseInvalid, version, found.TagName)
	}
	return found, nil
}

// signedManifest is a release's checksum manifest whose signature over the
// release version has been verified.
type signedManifest struct {
	version   string
	checksums []byte
	signature []byte
}

func (manifest signedManifest) write(dir string) error {
	if err := writeNew(filepath.Join(dir, ChecksumsName), manifest.checksums); err != nil {
		return err
	}
	return writeNew(filepath.Join(dir, SignatureName), manifest.signature)
}

// manifest downloads and verifies the release's checksum manifest before
// any binary is downloaded.
func (updater *Updater) manifest(ctx context.Context, value release) (signedManifest, error) {
	checksumAsset, err := findAsset(value.Assets, ChecksumsName)
	if err != nil {
		return signedManifest{}, err
	}
	signatureAsset, err := findAsset(value.Assets, SignatureName)
	if err != nil {
		return signedManifest{}, err
	}
	checksums, err := updater.downloadBytes(ctx, checksumAsset, MaxChecksumBytes)
	if err != nil {
		return signedManifest{}, fmt.Errorf("download %s: %w", ChecksumsName, err)
	}
	signature, err := updater.downloadBytes(ctx, signatureAsset, MaxSignatureBytes)
	if err != nil {
		return signedManifest{}, fmt.Errorf("download %s: %w", SignatureName, err)
	}
	if err := releasecontract.Verify(updater.publicKey, value.TagName, checksums, signature); err != nil {
		return signedManifest{}, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	return signedManifest{version: value.TagName, checksums: checksums, signature: signature}, nil
}

// Fetch downloads the named assets of release version with its checksum
// manifest and signature into dir, an existing directory the caller owns.
// The signature is verified before any asset is downloaded, and each asset's
// checksum as it arrives.
func (updater *Updater) Fetch(ctx context.Context, version string, names []string, dir string) error {
	if err := updater.keyReady(); err != nil {
		return err
	}
	value, err := updater.release(ctx, version)
	if err != nil {
		return err
	}
	signed, err := updater.manifest(ctx, value)
	if err != nil {
		return err
	}
	if err := signed.write(dir); err != nil {
		return err
	}
	for _, name := range names {
		found, err := findAsset(value.Assets, name)
		if err != nil {
			return err
		}
		expected, err := parseChecksum(signed.checksums, name)
		if err != nil {
			return err
		}
		if err := updater.downloadAsset(ctx, found, dir, expected, nil); err != nil {
			return err
		}
	}
	return nil
}

// downloadAsset writes one asset into dir under its own name and checks
// its checksum.
func (updater *Updater) downloadAsset(ctx context.Context, value asset, dir string, expected [sha256.Size]byte, report ProgressFunc) (err error) {
	path := filepath.Join(dir, value.Name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("stage %s: %w", value.Name, err)
	}
	defer func() {
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	actual, err := updater.downloadFile(ctx, value, file, MaxBinaryBytes, report)
	if err != nil {
		return fmt.Errorf("download %s: %w", value.Name, err)
	}
	if actual != expected {
		return fmt.Errorf("%w: %s expected %x, got %x", ErrChecksumInvalid, value.Name, expected, actual)
	}
	return file.Sync()
}

func (updater *Updater) downloadBytes(ctx context.Context, value asset, maximum int64) ([]byte, error) {
	request, err := updater.request(ctx, value.BrowserDownloadURL, "application/octet-stream", false)
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

func (updater *Updater) downloadFile(ctx context.Context, value asset, destination io.Writer, maximum int64, report ProgressFunc) ([sha256.Size]byte, error) {
	progress := Progress{Stage: StageDownload, Total: value.Size}
	report.emit(progress)
	request, err := updater.request(ctx, value.BrowserDownloadURL, "application/octet-stream", false)
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
	if response.ContentLength > 0 {
		progress.Total = response.ContentLength
	}
	writer := &downloadProgressWriter{writer: io.MultiWriter(destination, hash), report: report, progress: progress}
	written, err := io.Copy(writer, io.LimitReader(response.Body, maximum+1))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return [sha256.Size]byte{}, err
		}
		return [sha256.Size]byte{}, fmt.Errorf("%w: read asset: %w", ErrReleaseUnavailable, err)
	}
	if written > maximum {
		return [sha256.Size]byte{}, fmt.Errorf("%w: asset exceeds %d bytes", ErrReleaseInvalid, maximum)
	}
	if written == 0 {
		return [sha256.Size]byte{}, fmt.Errorf("%w: binary asset is empty", ErrReleaseInvalid)
	}
	progress.Downloaded = written
	progress.Total = written
	progress.Complete = true
	report.emit(progress)
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

// request builds a GET request. Only API requests carry the token: asset
// downloads are redirected to storage hosts that need none.
func (updater *Updater) request(ctx context.Context, rawURL, accept string, api bool) (*http.Request, error) {
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
	if api && updater.token != "" {
		request.Header.Set("Authorization", "Bearer "+updater.token)
	}
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
		base, baseErr := url.Parse(updater.releasesURL)
		if baseErr != nil || base.Scheme != "http" || base.Host != parsed.Host {
			return nil, fmt.Errorf("%w: insecure release URL %q", ErrReleaseInvalid, rawURL)
		}
	}
	return parsed, nil
}
