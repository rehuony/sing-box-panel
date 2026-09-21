// SPDX-License-Identifier: GPL-3.0-or-later
package catalog

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDecodeAssetHeavyReleasePage(t *testing.T) {
	// Each asset has nested uploader metadata just like GitHub. A valid page can
	// exceed the old 200,000-token limit while remaining far below the byte cap.
	asset := `{"id":1,"name":"other-platform.zip","uploader":{"login":"release-bot","id":2},"size":100,"digest":null}`
	body := `[{"id":1,"tag_name":"v1.14.0","assets":[` + strings.Repeat(asset+",", 12000) + asset + `]}]`
	var releases []githubRelease
	if err := decodeGitHubJSON([]byte(body), &releases); err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || len(releases[0].Assets) != 12001 {
		t.Fatalf("lost releases/assets: %+v", releases)
	}
}

func TestDecodeGitHubJSONRetainsStructuralValidation(t *testing.T) {
	for _, body := range []string{`[{"id":1,"id":2}]`, `[{"uploader":{"id":1,"id":2}}]`, `[] {}`, strings.Repeat("[", 65) + strings.Repeat("]", 65)} {
		var result any
		if err := decodeGitHubJSON([]byte(body), &result); err == nil {
			t.Fatalf("accepted malformed response: %.80s", body)
		}
	}
}

// A captured public response can be checked locally without making CI depend on GitHub.
func TestCapturedGitHubReleasePage(t *testing.T) {
	path := os.Getenv("SBP_CATALOG_RESPONSE")
	if path == "" {
		t.Skip("no captured GitHub response supplied")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var releases []githubRelease
	if err := decodeGitHubJSON(body, &releases); err != nil {
		t.Fatal(err)
	}
	assets := 0
	for _, release := range releases {
		assets += len(release.Assets)
	}
	if len(releases) == 0 || assets == 0 {
		t.Fatal("empty fixture")
	}
	t.Log(fmt.Sprintf("decoded %d bytes, %d releases, %d assets", len(body), len(releases), assets))
}
