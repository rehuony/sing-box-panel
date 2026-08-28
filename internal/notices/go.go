// SPDX-License-Identifier: GPL-3.0-or-later

package notices

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

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
