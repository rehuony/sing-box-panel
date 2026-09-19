// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
)

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
