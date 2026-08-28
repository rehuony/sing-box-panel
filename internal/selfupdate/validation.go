// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
