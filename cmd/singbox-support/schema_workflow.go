// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

const schemaAssetsPath = "internal/singbox/schemas"

type generatedSchemaManifest struct {
	SchemaVersion int                            `json:"schema_version"`
	Entries       []generatedSchemaManifestEntry `json:"entries"`
}

type generatedSchemaManifestEntry struct {
	ExactVersion string `json:"exact_version"`
	SchemaSHA256 string `json:"schema_sha256"`
	SchemaFile   string `json:"schema_file"`
	Source       string `json:"source"`
}

func exportWebSchemas(outputDirectory string) error {
	assets, err := singbox.ConfigurationSchemaWebAssets()
	if err != nil {
		return fmt.Errorf("load committed schema assets: %w", err)
	}
	for name, content := range assets {
		target := filepath.Join(outputDirectory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return fmt.Errorf("write Web schema asset %s: %w", name, err)
		}
	}
	return nil
}

func generateSchemas(root string) error {
	catalog, err := loadCatalog(filepath.Join(root, catalogPath))
	if err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("native schema regeneration must run on Linux; regular builds use committed assets")
	}
	temporaryRoot, err := os.MkdirTemp("", "sing-box-panel-schema-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryRoot)
	outputRoot := filepath.Join(temporaryRoot, "assets")
	manifest := generatedSchemaManifest{
		SchemaVersion: 1,
		Entries:       make([]generatedSchemaManifestEntry, 0),
	}
	for _, version := range catalog.Versions {
		var entry generatedSchemaManifestEntry
		var err error
		switch version.SchemaSource {
		case singbox.SchemaSourceNative:
			entry, err = generateNativeSchema(temporaryRoot, version, outputRoot)
		case singbox.SchemaSourceReviewed113:
			var raw []byte
			raw, err = canonicalJSONFile(filepath.Join(root, "cmd/singbox-support/schema-sources/1.13.json"))
			if err == nil {
				entry, err = writeConfigurationSchema(version, raw, outputRoot)
			}
		case "":
			continue
		default:
			return fmt.Errorf("unknown schema source %q", version.SchemaSource)
		}
		if err != nil {
			return err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestJSON = append(manifestJSON, '\n')
	if err := os.WriteFile(filepath.Join(outputRoot, "manifest.json"), manifestJSON, 0o644); err != nil {
		return err
	}
	fields, err := reviewedSchemaFields(root)
	if err != nil {
		return err
	}
	if err := writeFileIfChanged(filepath.Join(root, reviewedSchemaFieldsPath), fields); err != nil {
		return err
	}
	return syncSchemaAssets(outputRoot, filepath.Join(root, schemaAssetsPath))
}

func generateNativeSchema(
	temporaryRoot string,
	version singbox.Version,
	outputRoot string,
) (generatedSchemaManifestEntry, error) {
	profile, found := version.Profiles[runtime.GOARCH]
	if !found {
		return generatedSchemaManifestEntry{}, fmt.Errorf("sing-box %s has no linux/%s generation profile", version.ExactVersion, runtime.GOARCH)
	}
	binaryPath, err := downloadReleaseBinary(temporaryRoot, version, profile)
	if err != nil {
		return generatedSchemaManifestEntry{}, err
	}
	rawPath := filepath.Join(temporaryRoot, "schema-"+version.ExactVersion+".json")
	command := exec.Command(binaryPath, "schema", "-o", rawPath)
	command.Dir = temporaryRoot
	command.Env = []string{"HOME=" + temporaryRoot, "LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	if output, err := command.CombinedOutput(); err != nil {
		return generatedSchemaManifestEntry{}, fmt.Errorf("generate sing-box %s native schema: %w\n%s", version.ExactVersion, err, output)
	}
	raw, err := canonicalJSONFile(rawPath)
	if err != nil {
		return generatedSchemaManifestEntry{}, err
	}
	return writeConfigurationSchema(version, raw, outputRoot)
}

func writeConfigurationSchema(version singbox.Version, raw []byte, outputRoot string) (generatedSchemaManifestEntry, error) {
	schema, err := applyConfigurationSchemaPresentationOverlay(raw)
	if err != nil {
		return generatedSchemaManifestEntry{}, fmt.Errorf("apply presentation overlay to sing-box %s: %w", version.ExactVersion, err)
	}
	baseName := "schema-" + strings.ReplaceAll(version.ExactVersion, ".", "_") + ".json"
	schemaName := filepath.ToSlash(baseName)
	target := filepath.Join(outputRoot, filepath.FromSlash(schemaName))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return generatedSchemaManifestEntry{}, err
	}
	if err := os.WriteFile(target, schema, 0o644); err != nil {
		return generatedSchemaManifestEntry{}, err
	}
	return generatedSchemaManifestEntry{
		ExactVersion: version.ExactVersion,
		SchemaSHA256: digestHex(schema),
		SchemaFile:   schemaName,
		Source:       version.SchemaSource,
	}, nil
}

func downloadReleaseBinary(temporaryRoot string, version singbox.Version, profile singbox.Profile) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, profile.URL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "sing-box-panel-schema-generator")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("download sing-box %s release: %w", version.ExactVersion, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download sing-box %s release: unexpected HTTP status %s", version.ExactVersion, response.Status)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, profile.Size+1))
	if err != nil {
		return "", err
	}
	if int64(len(archive)) != profile.Size || digestHex(archive) != profile.SHA256 {
		return "", fmt.Errorf("downloaded sing-box %s release differs from the catalog", version.ExactVersion)
	}
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", fmt.Errorf("open sing-box %s release archive: %w", version.ExactVersion, err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	binaryPath := filepath.Join(temporaryRoot, "sing-box-"+version.ExactVersion)
	found := false
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read sing-box %s release archive: %w", version.ExactVersion, err)
		}
		cleanName := filepath.Clean(filepath.FromSlash(header.Name))
		if filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("sing-box %s release contains an unsafe path", version.ExactVersion)
		}
		if filepath.Base(cleanName) != "sing-box" {
			continue
		}
		if header.Typeflag != tar.TypeReg || found {
			return "", fmt.Errorf("sing-box %s release contains an invalid binary entry", version.ExactVersion)
		}
		binary, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil || int64(len(binary)) != header.Size {
			return "", fmt.Errorf("read sing-box %s release binary: %w", version.ExactVersion, err)
		}
		if err := os.WriteFile(binaryPath, binary, 0o755); err != nil {
			return "", err
		}
		found = true
	}
	if !found {
		return "", fmt.Errorf("sing-box %s release contains no binary", version.ExactVersion)
	}
	return binaryPath, nil
}

func canonicalJSONFile(filename string) ([]byte, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("schema output contains multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(decoded)
}

func syncSchemaAssets(source string, destination string) error {
	expected := make(map[string]bool)
	if err := filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		expected[relative] = true
		content, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return writeFileIfChanged(filepath.Join(destination, relative), content)
	}); err != nil {
		return err
	}
	var directories []string
	if err := filepath.WalkDir(destination, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != destination {
				directories = append(directories, current)
			}
			return nil
		}
		relative, err := filepath.Rel(destination, current)
		if err != nil {
			return err
		}
		if !expected[relative] {
			return os.Remove(current)
		}
		return nil
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		entries, err := os.ReadDir(directories[index])
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(directories[index]); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeFileIfChanged(filename string, content []byte) error {
	current, err := os.ReadFile(filename)
	if err == nil && bytes.Equal(current, content) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".schema-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filename)
}

func digestHex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

// Reviewed constraints are source data, independently maintained from x-panel.
// Offline checks ensure changing that source cannot leave committed assets stale.
func checkReviewedSchemas(root string, catalog sourceCatalog) error {
	expectedFields, err := reviewedSchemaFields(root)
	if err != nil {
		return err
	}
	currentFields, err := os.ReadFile(filepath.Join(root, reviewedSchemaFieldsPath))
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedFields, currentFields) {
		return errors.New("reviewed 1.13 field table is stale; run go tool singbox-support generate")
	}

	for _, version := range catalog.Versions {
		if version.SchemaSource != singbox.SchemaSourceReviewed113 {
			continue
		}
		raw, err := canonicalJSONFile(filepath.Join(root, "cmd/singbox-support/schema-sources/1.13.json"))
		if err != nil {
			return err
		}
		expected, err := applyConfigurationSchemaPresentationOverlay(raw)
		if err != nil {
			return err
		}
		contract, err := singbox.ConfigurationSchema(version.ExactVersion)
		if err != nil {
			return err
		}
		if !bytes.Equal(contract.Schema, expected) {
			return fmt.Errorf("reviewed schema %s is stale; run go tool singbox-support generate", version.ExactVersion)
		}
	}
	return nil
}
