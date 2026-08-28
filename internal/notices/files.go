// SPDX-License-Identifier: GPL-3.0-or-later

package notices

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

func findLicenseFiles(root string) ([]licenseFile, error) {
	var paths []string
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.HasPrefix(name, "license") || strings.HasPrefix(name, "licence") ||
			strings.HasPrefix(name, "copying") || strings.HasPrefix(name, "notice") ||
			strings.HasPrefix(name, "patents") {
			paths = append(paths, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no LICENSE, COPYING, NOTICE, or PATENTS file found")
	}
	slices.Sort(paths)
	mapped := make(map[string]string, len(paths))
	for _, path := range paths {
		mapped[path] = filepath.Join(root, filepath.FromSlash(path))
	}
	return readLicensePaths(mapped)
}

func readLicensePaths(paths map[string]string) ([]licenseFile, error) {
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	slices.Sort(names)
	files := make([]licenseFile, 0, len(names))
	for _, name := range names {
		content, err := os.ReadFile(paths[name])
		if err != nil {
			return nil, fmt.Errorf("read license document %s: %w", paths[name], err)
		}
		if len(content) == 0 {
			return nil, fmt.Errorf("license document %s is empty", paths[name])
		}
		if !utf8.Valid(content) {
			return nil, fmt.Errorf("license document %s is not UTF-8", paths[name])
		}
		digest := sha256.Sum256(content)
		files = append(files, licenseFile{
			Path:    name,
			Digest:  hex.EncodeToString(digest[:]),
			Content: content,
		})
	}
	return files, nil
}

func readGoDirective(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	match := goDirectivePattern.FindSubmatch(content)
	if len(match) != 2 {
		return "", fmt.Errorf("go.mod does not contain one supported go directive")
	}
	return string(match[1]), nil
}

func readPackageMetadata(path string) (packageMetadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return packageMetadata{}, fmt.Errorf("read package metadata %s: %w", path, err)
	}
	var metadata packageMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return packageMetadata{}, fmt.Errorf("decode package metadata %s: %w", path, err)
	}
	if metadata.Name == "" || metadata.Version == "" || metadata.License == "" {
		return packageMetadata{}, fmt.Errorf("package metadata %s has incomplete name, version, or license", path)
	}
	return metadata, nil
}

func firstExistingFile(paths ...string) (string, error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			return path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("none of the candidate paths exists: %s", strings.Join(paths, ", "))
}

func requirePathWithin(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return fmt.Errorf("compare package path with node_modules: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("package path %s is outside %s", candidate, root)
	}
	return nil
}

func validateHTTPURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return fmt.Errorf("expected an absolute HTTP(S) URL, got %q", value)
	}
	return nil
}
