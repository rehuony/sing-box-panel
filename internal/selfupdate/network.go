// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/rehuony/sing-box-panel/internal/releaseversion"
)

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
	if err := releaseversion.Validate(latest.TagName); err != nil {
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
