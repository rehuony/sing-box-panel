// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
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
	"go.yaml.in/yaml/v3":               "MIT AND Apache-2.0",
	"golang.org/x/mod":                 "BSD-3-Clause",
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
func generateNotices(ctx context.Context, root string) ([]byte, error) {
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
