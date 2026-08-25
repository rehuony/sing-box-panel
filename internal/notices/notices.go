// SPDX-License-Identifier: GPL-3.0-or-later

// Package notices builds the deterministic third-party notice distributed
// with sing-box-panel.
package notices

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var goDirectivePattern = regexp.MustCompile(`(?m)^go[ \t]+([0-9]+(?:\.[0-9]+){1,2})[ \t]*$`)

var goModuleLicenseIDs = map[string]string{
	"github.com/dustin/go-humanize":    "MIT",
	"github.com/google/uuid":           "BSD-3-Clause",
	"github.com/mattn/go-isatty":       "MIT",
	"github.com/ncruces/go-strftime":   "MIT",
	"github.com/remyoudompheng/bigfft": "BSD-3-Clause",
	"github.com/spf13/cobra":           "Apache-2.0",
	"github.com/spf13/pflag":           "BSD-3-Clause",
	"github.com/tailscale/hujson":      "BSD-3-Clause",
	"golang.org/x/sys":                 "BSD-3-Clause",
	"modernc.org/libc":                 "BSD-3-Clause",
	"modernc.org/mathutil":             "BSD-3-Clause",
	"modernc.org/memory":               "BSD-3-Clause",
	"modernc.org/sqlite":               "BSD-3-Clause",
}

// Web license identifiers are deliberately review-gated. A new production
// license must be inspected before it can be included in a release notice.
var webLicenseIDs = map[string]struct{}{
	"MIT": {},
}

type component struct {
	Ecosystem string
	Name      string
	Version   string
	Source    string
	LicenseID string
	Files     []licenseFile
}

func (value component) identity() string {
	return value.Ecosystem + ":" + value.Name + "@" + value.Version
}

type licenseFile struct {
	Path    string
	Digest  string
	Content []byte
}

type documentUse struct {
	Component string
	Path      string
}

type licenseDocument struct {
	Digest  string
	Content []byte
	Uses    []documentUse
}

type goPackage struct {
	Module *goModule
}

type goModule struct {
	Path    string
	Version string
	Dir     string
	Main    bool
	Replace *goModule
}

type pnpmLicenseEntry struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
	Paths    []string `json:"paths"`
	License  string   `json:"license"`
	Homepage string   `json:"homepage"`
}

type packageMetadata struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	License  string `json:"license"`
	Homepage string `json:"homepage"`
}

// Generate inspects the dependencies used by the two supported Linux release
// targets and returns the complete deterministic notice file.
func Generate(ctx context.Context, root string) ([]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	toolchain, err := collectGoToolchain(ctx, root)
	if err != nil {
		return nil, err
	}
	goComponents, err := collectGoModules(ctx, root)
	if err != nil {
		return nil, err
	}
	webComponents, err := collectWebPackages(ctx, root)
	if err != nil {
		return nil, err
	}

	components := make([]component, 0, 1+len(goComponents)+len(webComponents))
	components = append(components, toolchain)
	components = append(components, goComponents...)
	components = append(components, webComponents...)
	return render(components)
}

func collectGoToolchain(ctx context.Context, root string) (component, error) {
	version, err := readGoDirective(filepath.Join(root, "go.mod"))
	if err != nil {
		return component{}, err
	}
	output, err := commandOutput(ctx, root, nil, "go", "env", "GOROOT")
	if err != nil {
		return component{}, err
	}
	goRoot := strings.TrimSpace(string(output))
	if goRoot == "" {
		return component{}, fmt.Errorf("go env GOROOT returned an empty path")
	}

	licensePath, err := firstExistingFile(
		filepath.Join(goRoot, "LICENSE"),
		filepath.Join(filepath.Dir(goRoot), "LICENSE"),
	)
	if err != nil {
		return component{}, fmt.Errorf("locate Go toolchain LICENSE: %w", err)
	}
	patentsPath, err := firstExistingFile(
		filepath.Join(goRoot, "PATENTS"),
		filepath.Join(filepath.Dir(goRoot), "PATENTS"),
	)
	if err != nil {
		return component{}, fmt.Errorf("locate Go toolchain PATENTS: %w", err)
	}
	files, err := readLicensePaths(map[string]string{
		"LICENSE": licensePath,
		"PATENTS": patentsPath,
	})
	if err != nil {
		return component{}, err
	}
	return component{
		Ecosystem: "go-toolchain",
		Name:      "Go runtime and standard library",
		Version:   "go directive " + version,
		Source:    "https://go.dev/LICENSE",
		LicenseID: "BSD-3-Clause",
		Files:     files,
	}, nil
}

func collectGoModules(ctx context.Context, root string) ([]component, error) {
	modules := make(map[string]goModule)
	for _, architecture := range []string{"amd64", "arm64"} {
		output, err := commandOutput(ctx, root, []string{
			"CGO_ENABLED=0",
			"GOARCH=" + architecture,
			"GOENV=off",
			"GOEXPERIMENT=",
			"GOFLAGS=",
			"GOOS=linux",
			"GOTOOLCHAIN=local",
			"GOWORK=off",
		}, "go", "list", "-mod=readonly", "-deps", "-tags", "webdist", "-json", "./cmd/sing-box-panel")
		if err != nil {
			return nil, fmt.Errorf("list linux/%s dependencies: %w", architecture, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(output))
		for {
			var packageDescription goPackage
			if err := decoder.Decode(&packageDescription); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("decode go list output for linux/%s: %w", architecture, err)
			}
			module := packageDescription.Module
			if module == nil || module.Main {
				continue
			}
			if module.Replace != nil {
				return nil, fmt.Errorf("Go module %s uses an unsupported replacement", module.Path)
			}
			if module.Path == "" || module.Version == "" || module.Dir == "" {
				return nil, fmt.Errorf("Go dependency has incomplete identity: path=%q version=%q dir=%q", module.Path, module.Version, module.Dir)
			}
			key := module.Path + "@" + module.Version
			if existing, ok := modules[key]; ok && existing.Dir != module.Dir {
				return nil, fmt.Errorf("Go module %s resolved to both %s and %s", key, existing.Dir, module.Dir)
			}
			modules[key] = *module
		}
	}

	components := make([]component, 0, len(modules))
	for _, module := range modules {
		licenseID, ok := goModuleLicenseIDs[module.Path]
		if !ok {
			return nil, fmt.Errorf("Go module %s@%s has no reviewed license classification", module.Path, module.Version)
		}
		files, err := findLicenseFiles(module.Dir)
		if err != nil {
			return nil, fmt.Errorf("collect licenses for Go module %s@%s: %w", module.Path, module.Version, err)
		}
		components = append(components, component{
			Ecosystem: "go-module",
			Name:      module.Path,
			Version:   module.Version,
			Source:    "https://pkg.go.dev/" + module.Path + "@" + module.Version,
			LicenseID: licenseID,
			Files:     files,
		})
	}
	return components, nil
}

func collectWebPackages(ctx context.Context, root string) ([]component, error) {
	webRoot := filepath.Join(root, "web")
	modulesRoot, err := filepath.Abs(filepath.Join(webRoot, "node_modules"))
	if err != nil {
		return nil, fmt.Errorf("resolve web node_modules: %w", err)
	}
	output, err := commandOutput(ctx, webRoot, nil, "pnpm", "licenses", "list", "--prod", "--json")
	if err != nil {
		return nil, err
	}
	var report map[string][]pnpmLicenseEntry
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, fmt.Errorf("decode pnpm production license report: %w", err)
	}
	if len(report) == 0 {
		return nil, fmt.Errorf("pnpm production license report is empty")
	}

	componentsByIdentity := make(map[string]component)
	for licenseID, entries := range report {
		if _, ok := webLicenseIDs[licenseID]; !ok {
			return nil, fmt.Errorf("web production dependency uses an unreviewed license identifier %q", licenseID)
		}
		for _, entry := range entries {
			if entry.Name == "" || entry.License == "" || entry.Homepage == "" || len(entry.Versions) == 0 || len(entry.Paths) == 0 {
				return nil, fmt.Errorf("pnpm license entry is incomplete: name=%q license=%q homepage=%q", entry.Name, entry.License, entry.Homepage)
			}
			if entry.License != licenseID {
				return nil, fmt.Errorf("pnpm grouped %s under %s but declared %s", entry.Name, licenseID, entry.License)
			}
			if err := validateHTTPURL(entry.Homepage); err != nil {
				return nil, fmt.Errorf("pnpm package %s homepage: %w", entry.Name, err)
			}
			for _, packagePath := range entry.Paths {
				packagePath, err = filepath.Abs(packagePath)
				if err != nil {
					return nil, fmt.Errorf("resolve pnpm package %s path: %w", entry.Name, err)
				}
				if err := requirePathWithin(modulesRoot, packagePath); err != nil {
					return nil, fmt.Errorf("pnpm package %s: %w", entry.Name, err)
				}
				metadata, err := readPackageMetadata(filepath.Join(packagePath, "package.json"))
				if err != nil {
					return nil, err
				}
				if metadata.Name != entry.Name || metadata.License != licenseID || !slices.Contains(entry.Versions, metadata.Version) {
					return nil, fmt.Errorf("pnpm package metadata disagrees with license report for %s@%s", metadata.Name, metadata.Version)
				}
				files, err := findLicenseFiles(packagePath)
				if err != nil {
					return nil, fmt.Errorf("collect licenses for web package %s@%s: %w", metadata.Name, metadata.Version, err)
				}
				value := component{
					Ecosystem: "web-package",
					Name:      metadata.Name,
					Version:   metadata.Version,
					Source:    entry.Homepage,
					LicenseID: licenseID,
					Files:     files,
				}
				identity := value.identity()
				if existing, ok := componentsByIdentity[identity]; ok {
					if existing.Source != value.Source || existing.LicenseID != value.LicenseID ||
						!equalLicenseFiles(existing.Files, value.Files) {
						return nil, fmt.Errorf("conflicting pnpm license entries for %s", identity)
					}
					continue
				}
				componentsByIdentity[identity] = value
			}
		}
	}

	components := make([]component, 0, len(componentsByIdentity))
	for _, value := range componentsByIdentity {
		components = append(components, value)
	}
	return components, nil
}

func render(components []component) ([]byte, error) {
	if len(components) == 0 {
		return nil, fmt.Errorf("cannot render an empty third-party notice")
	}
	components = slices.Clone(components)
	slices.SortFunc(components, func(left, right component) int {
		if result := strings.Compare(left.Name, right.Name); result != 0 {
			return result
		}
		if result := strings.Compare(left.Version, right.Version); result != 0 {
			return result
		}
		return strings.Compare(left.Ecosystem, right.Ecosystem)
	})
	documents := make(map[string]*licenseDocument)
	for _, value := range components {
		if value.Name == "" || value.Version == "" || value.Source == "" || value.LicenseID == "" || len(value.Files) == 0 {
			return nil, fmt.Errorf("component %q has incomplete notice metadata", value.identity())
		}
		for _, file := range value.Files {
			document, ok := documents[file.Digest]
			if !ok {
				document = &licenseDocument{Digest: file.Digest, Content: file.Content}
				documents[file.Digest] = document
			} else if !bytes.Equal(document.Content, file.Content) {
				return nil, fmt.Errorf("SHA-256 collision between license documents %s", file.Digest)
			}
			document.Uses = append(document.Uses, documentUse{Component: value.identity(), Path: file.Path})
		}
	}

	var output bytes.Buffer
	output.WriteString("THIRD-PARTY NOTICES\n\n")
	output.WriteString("Generated by: go run ./cmd/third-party-notices\n")
	output.WriteString("Do not edit this file manually. Regenerate it after dependency changes.\n\n")
	output.WriteString("SCOPE\n")
	output.WriteString("This notice covers the Go runtime and standard library, every external Go module\n")
	output.WriteString("linked into the supported linux/amd64 or linux/arm64 webdist binary, and every\n")
	output.WriteString("production package in the bundled Web application. Build-only and test-only Web\n")
	output.WriteString("packages are not distributed and are intentionally excluded.\n\n")
	output.WriteString("EXTERNAL RUNTIME: SING-BOX\n")
	output.WriteString("sing-box-panel downloads verified sing-box artifacts directly from the official\n")
	output.WriteString("SagerNet release repository and runs sing-box as a separate process. The current\n")
	output.WriteString("sing-box-panel release artifacts do not copy, link, or bundle the sing-box binary.\n")
	output.WriteString("Upstream: https://github.com/SagerNet/sing-box\n")
	output.WriteString("Copyright: Copyright (C) 2022 by nekohasekai <contact-sagernet@sekai.icu>\n")
	output.WriteString("License: GPL-3.0-or-later (the GPLv3 text is provided in COPYING)\n")
	output.WriteString("Additional upstream term: In addition, no derivative work may use the name or\n")
	output.WriteString("imply association with this application without prior consent.\n\n")
	output.WriteString("INCLUDED COMPONENTS\n\n")
	for _, value := range components {
		fmt.Fprintf(&output, "Component: %s\n", value.Name)
		fmt.Fprintf(&output, "Ecosystem: %s\n", value.Ecosystem)
		fmt.Fprintf(&output, "Version: %s\n", value.Version)
		fmt.Fprintf(&output, "Source: %s\n", value.Source)
		fmt.Fprintf(&output, "License: %s\n", value.LicenseID)
		output.WriteString("License documents:\n")
		for _, file := range value.Files {
			fmt.Fprintf(&output, "  - %s (SHA-256: %s)\n", file.Path, file.Digest)
		}
		output.WriteByte('\n')
	}

	documentList := make([]*licenseDocument, 0, len(documents))
	for _, document := range documents {
		slices.SortFunc(document.Uses, func(left, right documentUse) int {
			if result := strings.Compare(left.Component, right.Component); result != 0 {
				return result
			}
			return strings.Compare(left.Path, right.Path)
		})
		documentList = append(documentList, document)
	}
	slices.SortFunc(documentList, func(left, right *licenseDocument) int {
		return strings.Compare(left.Digest, right.Digest)
	})

	output.WriteString("LICENSE TEXTS\n\n")
	for _, document := range documentList {
		fmt.Fprintf(&output, "================================================================================\n")
		fmt.Fprintf(&output, "SHA-256: %s\n", document.Digest)
		output.WriteString("Applies to:\n")
		for _, use := range document.Uses {
			fmt.Fprintf(&output, "  - %s — %s\n", use.Component, use.Path)
		}
		output.WriteString("--------------------------------------------------------------------------------\n")
		output.Write(document.Content)
		if !bytes.HasSuffix(document.Content, []byte("\n")) {
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

func equalLicenseFiles(left, right []licenseFile) bool {
	return slices.EqualFunc(left, right, func(leftFile, rightFile licenseFile) bool {
		return leftFile.Path == rightFile.Path && leftFile.Digest == rightFile.Digest &&
			bytes.Equal(leftFile.Content, rightFile.Content)
	})
}

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

func commandOutput(ctx context.Context, directory string, overrides []string, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = directory
	command.Env = mergedEnvironment(overrides)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("run %s %s: %w: %s", name, strings.Join(arguments, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func mergedEnvironment(overrides []string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	for _, entry := range overrides {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}
