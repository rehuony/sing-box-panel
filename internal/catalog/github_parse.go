// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func stableVersion(tag string) (coreartifact.ExactVersion, bool) {
	if len(tag) < 2 || tag[0] != 'v' {
		return coreartifact.ExactVersion{}, false
	}
	version, err := coreartifact.ParseExactVersion(tag[1:])
	if err != nil || version.IsZero() {
		return coreartifact.ExactVersion{}, false
	}
	return version, true
}

func classifyAsset(version coreartifact.ExactVersion, name string) (coreartifact.Architecture, coreartifact.Variant, bool) {
	prefix := "sing-box-" + version.String() + "-linux-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".tar.gz") || containsControl(name) {
		return "", "", false
	}
	platform := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".tar.gz")
	switch platform {
	case "amd64":
		return coreartifact.ArchitectureAMD64, coreartifact.VariantPlain, true
	case "amd64-glibc":
		return coreartifact.ArchitectureAMD64, coreartifact.VariantGlibc, true
	case "amd64-musl":
		return coreartifact.ArchitectureAMD64, coreartifact.VariantMusl, true
	case "arm64":
		return coreartifact.ArchitectureARM64, coreartifact.VariantPlain, true
	case "arm64-glibc":
		return coreartifact.ArchitectureARM64, coreartifact.VariantGlibc, true
	case "arm64-musl":
		return coreartifact.ArchitectureARM64, coreartifact.VariantMusl, true
	}
	if strings.HasPrefix(platform, "amd64v") && allDigits(strings.TrimPrefix(platform, "amd64v")) {
		return coreartifact.ArchitectureAMD64, coreartifact.Variant(platform), true
	}
	return "", "", false
}

func validOfficialDownloadURL(rawURL string, version coreartifact.ExactVersion, assetName string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "github.com" || parsed.User != nil || parsed.Port() != "" || parsed.Fragment != "" || parsed.RawQuery != "" {
		return false
	}
	expectedPath := "/SagerNet/sing-box/releases/download/v" + version.String() + "/" + assetName
	return parsed.Path == expectedPath && parsed.EscapedPath() == expectedPath
}

func parseGitHubDigest(value string) (coreartifact.SHA256, error) {
	algorithm, encoded, found := strings.Cut(value, ":")
	if !found || algorithm != "sha256" {
		return coreartifact.SHA256{}, fmt.Errorf("unsupported digest algorithm")
	}
	return coreartifact.ParseSHA256(encoded)
}

func hasNextLink(value string) bool {
	for _, link := range strings.Split(value, ",") {
		parts := strings.Split(link, ";")
		for _, parameter := range parts[1:] {
			if strings.TrimSpace(parameter) == `rel="next"` {
				return true
			}
		}
	}
	return false
}

func allDigits(value string) bool {
	if value == "" || len(value) > 3 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func decodeGitHubJSON(data []byte, destination any) error {
	if err := rejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	stack := make([]map[string]struct{}, 0, 16)
	expectingKey := make([]bool, 0, 16)
	tokens := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		tokens++
		if tokens > 200_000 {
			return fmt.Errorf("JSON token limit exceeded")
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, make(map[string]struct{}))
				expectingKey = append(expectingKey, true)
			case '[':
				stack = append(stack, nil)
				expectingKey = append(expectingKey, false)
			case '}', ']':
				if len(stack) == 0 {
					return fmt.Errorf("unmatched JSON delimiter")
				}
				stack = stack[:len(stack)-1]
				expectingKey = expectingKey[:len(expectingKey)-1]
				markValueConsumed(stack, expectingKey)
			}
			if len(stack) > 64 {
				return fmt.Errorf("JSON nesting limit exceeded")
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1] != nil && expectingKey[len(expectingKey)-1] {
				object := stack[len(stack)-1]
				if _, duplicate := object[value]; duplicate {
					return fmt.Errorf("duplicate JSON key")
				}
				object[value] = struct{}{}
				expectingKey[len(expectingKey)-1] = false
			} else {
				markValueConsumed(stack, expectingKey)
			}
		default:
			markValueConsumed(stack, expectingKey)
		}
	}
}

func markValueConsumed(stack []map[string]struct{}, expectingKey []bool) {
	if len(stack) > 0 && stack[len(stack)-1] != nil && !expectingKey[len(expectingKey)-1] {
		expectingKey[len(expectingKey)-1] = true
	}
}
