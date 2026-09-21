// SPDX-License-Identifier: GPL-3.0-or-later

// Command singbox-support generates and checks sing-box support assets.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/singbox"
)

const (
	catalogPath   = "internal/singbox/catalog.json"
	generatedPath = "internal/singbox/catalog_generated.go"
)

type sourceCatalog struct {
	SchemaVersion    int               `json:"schema_version"`
	SourceRepository string            `json:"source_repository"`
	Versions         []singbox.Version `json:"versions"`
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: singbox-support generate|check|export-web --out DIRECTORY")
	}
	root, err := repositoryRoot()
	if err != nil {
		fatalf("locate repository: %v", err)
	}
	switch os.Args[1] {
	case "generate":
		if len(os.Args) != 2 {
			fatalf("usage: singbox-support generate")
		}
		err = generate(root)
	case "check":
		if len(os.Args) != 2 {
			fatalf("usage: singbox-support check")
		}
		err = check(root)
	case "export-web":
		if len(os.Args) != 4 || os.Args[2] != "--out" || strings.TrimSpace(os.Args[3]) == "" {
			fatalf("usage: singbox-support export-web --out DIRECTORY")
		}
		err = exportWebSchemas(os.Args[3])
	default:
		fatalf("usage: singbox-support generate|check|export-web --out DIRECTORY")
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func repositoryRoot() (string, error) {
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("go.mod was not found in this directory or its parents")
		}
		directory = parent
	}
}

func generate(root string) error {
	if err := generateCatalog(root); err != nil {
		return err
	}
	return generateSchemas(root)
}

func generateCatalog(root string) error {
	catalog, err := loadCatalog(filepath.Join(root, catalogPath))
	if err != nil {
		return err
	}
	generated, err := renderGenerated(catalog)
	if err != nil {
		return err
	}
	path := filepath.Join(root, generatedPath)
	current, readErr := os.ReadFile(path)
	if readErr == nil && bytes.Equal(current, generated) {
		return nil
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := os.WriteFile(path, generated, 0o644); err != nil {
		return fmt.Errorf("write generated support catalog: %w", err)
	}
	return nil
}

func check(root string) error {
	catalog, err := loadCatalog(filepath.Join(root, catalogPath))
	if err != nil {
		return err
	}
	generated, err := renderGenerated(catalog)
	if err != nil {
		return err
	}
	onDisk, err := os.ReadFile(filepath.Join(root, generatedPath))
	if err != nil {
		return fmt.Errorf("read generated support catalog: %w", err)
	}
	if !bytes.Equal(onDisk, generated) {
		return errors.New("generated support catalog is stale; run `go tool singbox-support generate`")
	}
	if err := singbox.ValidateConfigurationSchemaAssets(); err != nil {
		return fmt.Errorf("validate configuration schema assets: %w", err)
	}
	if err := singbox.ValidateFamilies(); err != nil {
		return err
	}
	expectedInboundVersions := make([]string, 0, len(catalog.Versions))
	for _, version := range catalog.Versions {
		if version.InboundFamily != "" {
			expectedInboundVersions = append(expectedInboundVersions, version.ExactVersion)
		}
	}
	if got := singbox.NewInboundRegistry().Versions(); !slices.Equal(got, expectedInboundVersions) {
		return fmt.Errorf("inbound registry versions %v differ from catalog %v", got, expectedInboundVersions)
	}
	return nil
}

func loadCatalog(path string) (sourceCatalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sourceCatalog{}, fmt.Errorf("read support catalog: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var catalog sourceCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return sourceCatalog{}, fmt.Errorf("decode support catalog: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return sourceCatalog{}, errors.New("support catalog contains multiple JSON values")
		}
		return sourceCatalog{}, fmt.Errorf("decode support catalog trailer: %w", err)
	}
	if err := validateCatalog(catalog); err != nil {
		return sourceCatalog{}, err
	}
	return catalog, nil
}

func validateCatalog(catalog sourceCatalog) error {
	if catalog.SchemaVersion != 1 {
		return fmt.Errorf("unsupported catalog schema version %d", catalog.SchemaVersion)
	}
	if catalog.SourceRepository != "SagerNet/sing-box" {
		return errors.New("source_repository must be SagerNet/sing-box")
	}
	if len(catalog.Versions) == 0 {
		return errors.New("support catalog has no versions")
	}
	familyContracts := make(map[string]string)
	var previous coreartifact.ExactVersion
	for index, version := range catalog.Versions {
		parsed, err := exactVersion(version.ExactVersion)
		if err != nil {
			return fmt.Errorf("versions[%d]: %w", index, err)
		}
		if index > 0 && previous.Compare(parsed) >= 0 {
			return errors.New("support versions must be unique and strictly ascending")
		}
		previous = parsed
		releaseLine := fmt.Sprintf("%d.%d", parsed.Major(), parsed.Minor())
		if version.InboundFamily != "" && !validBehaviorFamily(version.InboundFamily, releaseLine) {
			return fmt.Errorf("version %s has mismatched inbound family %s", version.ExactVersion, version.InboundFamily)
		}
		if version.Upstream.Tag != "v"+version.ExactVersion || !isLowerHex(version.Upstream.Commit, 40) ||
			!validModuleSum(version.Upstream.ModuleSum) || !validModuleSum(version.Upstream.GoModSum) {
			return fmt.Errorf("version %s has invalid upstream provenance", version.ExactVersion)
		}
		if len(version.Profiles) != 2 {
			return fmt.Errorf("version %s must have exactly amd64 and arm64 profiles", version.ExactVersion)
		}
		for _, architecture := range []string{singbox.ArchitectureAMD64, singbox.ArchitectureARM64} {
			profile, ok := version.Profiles[architecture]
			if !ok {
				return fmt.Errorf("version %s is missing %s profile", version.ExactVersion, architecture)
			}
			expectedAsset := fmt.Sprintf("sing-box-%s-linux-%s-musl.tar.gz", version.ExactVersion, architecture)
			expectedURL := fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/v%s/%s", version.ExactVersion, expectedAsset)
			if profile.AssetName != expectedAsset || profile.URL != expectedURL || !isLowerHex(profile.SHA256, 64) || profile.Size <= 0 {
				return fmt.Errorf("version %s has invalid %s asset metadata", version.ExactVersion, architecture)
			}
			if len(profile.Features) == 0 || !slices.IsSorted(profile.Features) || hasDuplicate(profile.Features) {
				return fmt.Errorf("version %s %s features must be non-empty, unique, and sorted", version.ExactVersion, architecture)
			}
			if !slices.Contains(profile.Features, "with_musl") {
				return fmt.Errorf("version %s %s profile must include with_musl", version.ExactVersion, architecture)
			}
			for _, feature := range profile.Features {
				if !validBuildTag(feature) {
					return fmt.Errorf("version %s %s has unsafe build tag %q", version.ExactVersion, architecture, feature)
				}
			}
		}
		if version.InboundFamily != "" {
			contract, _ := json.Marshal(struct {
				AMD64 []string
				ARM64 []string
			}{
				AMD64: version.Profiles[singbox.ArchitectureAMD64].Features,
				ARM64: version.Profiles[singbox.ArchitectureARM64].Features,
			})
			if expected, ok := familyContracts[version.InboundFamily]; ok && expected != string(contract) {
				return fmt.Errorf("version %s changes the reviewed contract inside inbound family %s", version.ExactVersion, version.InboundFamily)
			}
			familyContracts[version.InboundFamily] = string(contract)
		}
	}
	return nil
}

func renderGenerated(catalog sourceCatalog) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("// Code generated by go tool singbox-support generate; DO NOT EDIT.\n")
	output.WriteString("// SPDX-License-Identifier: GPL-3.0-or-later\n\npackage singbox\n\n")
	output.WriteString("var generatedVersions = []Version{\n")
	for _, version := range catalog.Versions {
		fmt.Fprintf(&output, "{ExactVersion:%q, InboundFamily:%q, Upstream:Upstream{Tag:%q, Commit:%q, ModuleSum:%q, GoModSum:%q}, Profiles:map[string]Profile{\n",
			version.ExactVersion, version.InboundFamily, version.Upstream.Tag, version.Upstream.Commit,
			version.Upstream.ModuleSum, version.Upstream.GoModSum)
		for _, architecture := range []string{singbox.ArchitectureAMD64, singbox.ArchitectureARM64} {
			profile := version.Profiles[architecture]
			fmt.Fprintf(&output, "%q:{AssetName:%q, URL:%q, SHA256:%q, Size:%d, Features:%#v},\n",
				architecture, profile.AssetName, profile.URL, profile.SHA256, profile.Size, profile.Features)
		}
		output.WriteString("}},\n")
	}
	output.WriteString("}\n")
	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated support catalog: %w\n%s", err, output.Bytes())
	}
	return formatted, nil
}

func exactVersion(value string) (coreartifact.ExactVersion, error) {
	parsed, err := coreartifact.ParseExactVersion(value)
	if err != nil || parsed.IsZero() || parsed.String() != value {
		return coreartifact.ExactVersion{}, fmt.Errorf("invalid stable exact version %q", value)
	}
	return parsed, nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validModuleSum(value string) bool {
	if !strings.HasPrefix(value, "h1:") || len(value) < 8 {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "h1:") {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '+' || character == '/' || character == '=' {
			continue
		}
		return false
	}
	return true
}

func hasDuplicate(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}
	return false
}

func validBuildTag(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '_' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validBehaviorFamily(value, releaseLine string) bool {
	if value == releaseLine {
		return true
	}
	if !strings.HasPrefix(value, releaseLine+"-") || len(value) > 64 {
		return false
	}
	suffix := strings.TrimPrefix(value, releaseLine+"-")
	if suffix == "" || suffix[0] == '-' || suffix[len(suffix)-1] == '-' || strings.Contains(suffix, "--") {
		return false
	}
	for _, character := range suffix {
		if character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func fatalf(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "singbox-support: "+format+"\n", arguments...)
	os.Exit(1)
}
