// SPDX-License-Identifier: GPL-3.0-or-later

package notices

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderIsDeterministicAndDeduplicatesLicenseText(t *testing.T) {
	sharedText := []byte("shared license\n")
	sharedDigest := digestForTest(sharedText)
	components := []component{
		{
			Ecosystem: "web-package",
			Name:      "z-example-web",
			Version:   "1.0.0",
			Source:    "https://example.com/web",
			LicenseID: "MIT",
			Files:     []licenseFile{{Path: "LICENSE", Digest: sharedDigest, Content: sharedText}},
		},
		{
			Ecosystem: "go-module",
			Name:      "a.example/go",
			Version:   "v1.0.0",
			Source:    "https://example.com/go",
			LicenseID: "MIT",
			Files:     []licenseFile{{Path: "COPYING", Digest: sharedDigest, Content: sharedText}},
		},
	}

	first, err := render(components)
	if err != nil {
		t.Fatalf("render(first): %v", err)
	}
	second, err := render(components)
	if err != nil {
		t.Fatalf("render(second): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("render output changed between identical calls")
	}
	if count := strings.Count(string(first), "shared license\n"); count != 1 {
		t.Fatalf("shared license text count = %d, want 1", count)
	}
	if !bytes.Contains(first, []byte("go-module:a.example/go@v1.0.0 — COPYING")) ||
		!bytes.Contains(first, []byte("web-package:z-example-web@1.0.0 — LICENSE")) {
		t.Fatalf("deduplicated license uses missing from output:\n%s", first)
	}
	if goOffset, webOffset := bytes.Index(first, []byte("Component: a.example/go")), bytes.Index(first, []byte("Component: z-example-web")); goOffset < 0 || webOffset < 0 || goOffset > webOffset {
		t.Fatalf("components are not sorted by name:\n%s", first)
	}
}

func TestRenderRejectsDigestCollision(t *testing.T) {
	components := []component{
		{
			Ecosystem: "go-module",
			Name:      "example.com/one",
			Version:   "v1.0.0",
			Source:    "https://example.com/one",
			LicenseID: "MIT",
			Files:     []licenseFile{{Path: "LICENSE", Digest: "same", Content: []byte("one")}},
		},
		{
			Ecosystem: "go-module",
			Name:      "example.com/two",
			Version:   "v1.0.0",
			Source:    "https://example.com/two",
			LicenseID: "MIT",
			Files:     []licenseFile{{Path: "LICENSE", Digest: "same", Content: []byte("two")}},
		},
	}

	if _, err := render(components); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("render collision error = %v", err)
	}
}

func TestFindLicenseFilesIncludesNestedDocuments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "LICENSE"), []byte("root license\n"), 0o644); err != nil {
		t.Fatalf("write root license: %v", err)
	}
	nested := filepath.Join(root, "third_party")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "NOTICE.txt"), []byte("nested notice\n"), 0o644); err != nil {
		t.Fatalf("write nested notice: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	files, err := findLicenseFiles(root)
	if err != nil {
		t.Fatalf("findLicenseFiles: %v", err)
	}
	if len(files) != 2 || files[0].Path != "LICENSE" || files[1].Path != "third_party/NOTICE.txt" {
		t.Fatalf("license paths = %#v", files)
	}
}

func TestFindLicenseFilesRejectsMissingDocuments(t *testing.T) {
	if _, err := findLicenseFiles(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no LICENSE") {
		t.Fatalf("findLicenseFiles error = %v", err)
	}
}

func TestReadGoDirective(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(path, []byte("module example.com/project\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	version, err := readGoDirective(path)
	if err != nil {
		t.Fatalf("readGoDirective: %v", err)
	}
	if version != "1.25.0" {
		t.Fatalf("version = %q, want 1.25.0", version)
	}
}

func TestRequirePathWithin(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "package")
	if err := requirePathWithin(root, inside); err != nil {
		t.Fatalf("inside path rejected: %v", err)
	}
	if err := requirePathWithin(root, filepath.Join(filepath.Dir(root), "outside")); err == nil {
		t.Fatal("outside path accepted")
	}
}

func digestForTest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
