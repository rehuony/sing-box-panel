// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	panelCommandPath = "github.com/rehuony/sing-box-panel/cmd/sing-box-panel"
	panelModulePath  = "github.com/rehuony/sing-box-panel"
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

func executableDigest(path string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	file, err := os.Open(path)
	if err != nil {
		return digest, fmt.Errorf("%w: open for identity check: %w", ErrExecutableInvalid, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return digest, fmt.Errorf("%w: inspect for identity check: %w", ErrExecutableInvalid, err)
	}
	if !info.Mode().IsRegular() {
		return digest, fmt.Errorf("%w: path must remain a regular file", ErrExecutableInvalid)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return digest, fmt.Errorf("%w: hash current executable: %w", ErrExecutableInvalid, err)
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func requireExecutableDigest(path string, expected [sha256.Size]byte) error {
	actual, err := executableDigest(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return ErrExecutableChanged
	}
	return nil
}

func validateStagedExecutable(path, expectedGOOS, expectedGOARCH string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: read Go build information: %v", ErrStagedExecutableInvalid, err)
	}
	return validateStagedBuildIdentity(info, expectedGOOS, expectedGOARCH)
}

func validateStagedBuildIdentity(info *buildinfo.BuildInfo, expectedGOOS, expectedGOARCH string) error {
	if info == nil || info.Path != panelCommandPath || info.Main.Path != panelModulePath {
		return fmt.Errorf("%w: unexpected Go command or module path", ErrStagedExecutableInvalid)
	}
	wantedSettings := map[string]string{
		"CGO_ENABLED": "0",
		"GOARCH":      expectedGOARCH,
		"GOOS":        expectedGOOS,
	}
	seen := make(map[string]string, len(wantedSettings))
	for _, setting := range info.Settings {
		if _, wanted := wantedSettings[setting.Key]; !wanted {
			continue
		}
		if _, duplicate := seen[setting.Key]; duplicate {
			return fmt.Errorf("%w: duplicate %s build setting", ErrStagedExecutableInvalid, setting.Key)
		}
		seen[setting.Key] = setting.Value
	}
	for key, expected := range wantedSettings {
		actual, found := seen[key]
		if !found || actual != expected {
			return fmt.Errorf("%w: unexpected %s build setting", ErrStagedExecutableInvalid, key)
		}
	}
	return nil
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
