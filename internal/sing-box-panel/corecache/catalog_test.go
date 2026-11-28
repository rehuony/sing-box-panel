// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestAssetValidationIgnoresDigestMetadata(t *testing.T) {
	t.Parallel()
	for _, metadata := range []Asset{
		{}, {HasAPIDigest: true}, {APIDigest: digest(t, "11")},
		{APIDigest: digest(t, "11"), HasAPIDigest: true, CatalogDigest: digest(t, "22"), HasCatalogDigest: true},
	} {
		metadata.RepositoryID = OfficialRepositoryID
		metadata.ReleaseID, metadata.AssetID = 1, 2
		metadata.Name = "sing-box-1.13.19-linux-amd64-musl.tar.gz"
		metadata.DownloadURL = "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/" + metadata.Name
		metadata.Size = 100
		metadata.Version = exactVersion(t, "1.13.19")
		metadata.OperatingSystem, metadata.Architecture, metadata.Variant = coreartifact.OperatingSystemLinux, coreartifact.ArchitectureAMD64, coreartifact.VariantMusl
		if err := metadata.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStableVersionAndAssetClassification(t *testing.T) {
	t.Parallel()
	validVersion := exactVersion(t, "1.13.19")
	versionTests := []struct {
		tag   string
		valid bool
	}{
		{tag: "v1.13.19", valid: true},
		{tag: "1.13.19"},
		{tag: "v1.14.0-beta.1"},
		{tag: "v01.13.19"},
		{tag: "v1.13.19+build"},
		{tag: "v0.0.0"},
	}
	for _, test := range versionTests {
		_, valid := stableVersion(test.tag)
		if valid != test.valid {
			t.Fatalf("stableVersion(%q) valid = %t, want %t", test.tag, valid, test.valid)
		}
	}

	assetTests := []struct {
		name    string
		arch    coreartifact.Architecture
		variant coreartifact.Variant
		valid   bool
	}{
		{name: "sing-box-1.13.19-linux-amd64.tar.gz"},
		{name: "sing-box-1.13.19-linux-amd64-glibc.tar.gz"},
		{name: "sing-box-1.13.19-linux-amd64v3.tar.gz"},
		{name: "sing-box-1.13.19-linux-amd64-musl.tar.gz", arch: coreartifact.ArchitectureAMD64, variant: coreartifact.VariantMusl, valid: true},
		{name: "sing-box-1.13.19-linux-arm64-musl.tar.gz", arch: coreartifact.ArchitectureARM64, variant: coreartifact.VariantMusl, valid: true},
		{name: "sing-box-1.13.19-linux-386.tar.gz"},
		{name: "sing-box-1.13.18-linux-amd64.tar.gz"},
		{name: "sing-box-1.13.19-linux-amd64.zip"},
	}
	for _, test := range assetTests {
		architecture, variant, valid := classifyAsset(validVersion, test.name)
		if valid != test.valid || architecture != test.arch || variant != test.variant {
			t.Fatalf("classifyAsset(%q) = (%q, %q, %t), want (%q, %q, %t)", test.name, architecture, variant, valid, test.arch, test.variant, test.valid)
		}
	}

	exactURL := "https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz"
	if !validOfficialDownloadURL(exactURL, validVersion, assetTests[0].name) {
		t.Fatalf("validOfficialDownloadURL rejected exact GitHub asset URL")
	}
	for _, invalidURL := range []string{
		"https://github.com/SagerNet/sing-box/releases/download/v1.13.18/sing-box-1.13.19-linux-amd64.tar.gz",
		"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/other.tar.gz",
		"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz?token=unexpected",
	} {
		if validOfficialDownloadURL(invalidURL, validVersion, assetTests[0].name) {
			t.Fatalf("validOfficialDownloadURL accepted %q", invalidURL)
		}
	}
}

func TestGitHubRefreshReadsSupportedStableReleasesFromTags(t *testing.T) {
	t.Parallel()
	apiDigest := digest(t, "11")
	// GitHub lists tags newest version first. The second page holds no tag of
	// a supported line, so it ends discovery although GitHub has more pages.
	pageOne := `[{"name":"v1.14.2-beta.1"},{"name":"v1.14.1"},{"name":"v1.13.19"},{"name":"v1.13.18"}]`
	pageTwo := `[{"name":"v1.13.0-beta.2"},{"name":"v1.12.3"},{"name":"latest"}]`
	releaseOne := `{"id":107,"tag_name":"v1.14.1","draft":false,"prerelease":false,"assets":[{"id":1006,"name":"sing-box-1.14.1-linux-amd64-musl.tar.gz","size":104,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.14.1/sing-box-1.14.1-linux-amd64-musl.tar.gz"}]}`
	releaseTwo := `{"id":101,"tag_name":"v1.13.19","draft":false,"prerelease":false,"unknown":"accepted","assets":[
		{"id":1001,"name":"sing-box-1.13.19-linux-amd64-musl.tar.gz","size":100,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-musl.tar.gz","digest":"sha256:` + apiDigest.String() + `"},
		{"id":1002,"name":"sing-box-1.13.19-linux-arm64-musl.tar.gz","size":101,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-arm64-musl.tar.gz","digest":"invalid-upstream-digest"},
		{"id":1003,"name":"sing-box-1.13.19-linux-amd64-glibc.tar.gz","size":102,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-glibc.tar.gz"},
		{"id":1004,"name":"sing-box-1.13.19-windows-amd64.zip","size":103,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-windows-amd64.zip"}
	]}`
	doer := &queueDoer{responses: []*http.Response{
		jsonResponse(http.StatusOK, pageOne, tagPageHeaders(`W/"tags-1"`, true)),
		jsonResponse(http.StatusOK, pageTwo, tagPageHeaders(`W/"tags-2"`, true)),
		jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil),
		jsonResponse(http.StatusOK, releaseOne, nil),
		jsonResponse(http.StatusOK, releaseTwo, nil),
	}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer, Token: "secret-token"})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err := client.Refresh(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.NotModified || result.Catalog.RepositoryID != OfficialRepositoryID {
		t.Fatalf("refresh metadata = %+v", result)
	}
	pageETags, err := client.decodeTagValidator(result.ETag)
	if err != nil || len(pageETags) != 2 || pageETags[0] != `W/"tags-1"` || pageETags[1] != `W/"tags-2"` {
		t.Fatalf("tag validator = (%q, %v)", pageETags, err)
	}
	// 1.13.18 lies before the first supported line and prereleases stay out,
	// so only the two supported stable tags have their releases read.
	if len(result.Catalog.Releases) != 2 || result.Catalog.Releases[0].Version.String() != "1.14.1" || result.Catalog.Releases[1].Version.String() != "1.13.19" {
		t.Fatalf("filtered releases = %+v", result.Catalog.Releases)
	}
	assets := result.Catalog.Releases[1].Assets
	if len(assets) != 2 {
		t.Fatalf("stable musl Linux assets = %d, want 2", len(assets))
	}
	assetsByID := make(map[int64]Asset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.AssetID] = asset
	}
	if assetsByID[1001].APIDigest != apiDigest || assetsByID[1002].HasAPIDigest {
		t.Fatal("unexpected informational digest metadata")
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Message, "secret-token") || strings.Contains(diagnostic.Code, "digest") {
			t.Fatalf("unexpected diagnostic: %+v", diagnostic)
		}
	}
	requests := doer.Requests()
	if len(requests) != 5 {
		t.Fatalf("request count = %d, want two tag pages, the repository and two releases", len(requests))
	}
	if requests[0].URL.Path != "/repos/SagerNet/sing-box/tags" || !strings.Contains(requests[0].URL.RawQuery, "per_page=100") ||
		requests[0].Header.Get("If-None-Match") != "" {
		t.Fatalf("first tag request = %s %v", requests[0].URL, requests[0].Header)
	}
	if requests[3].URL.Path != "/repos/SagerNet/sing-box/releases/tags/v1.14.1" || requests[4].URL.Path != "/repos/SagerNet/sing-box/releases/tags/v1.13.19" {
		t.Fatalf("release requests = %s, %s", requests[3].URL.Path, requests[4].URL.Path)
	}
	if got := requests[0].Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestGitHubRefreshReadsNothingMoreWhileNoStableTagChanged(t *testing.T) {
	t.Parallel()
	previous := &Catalog{RepositoryID: OfficialRepositoryID, Releases: []Release{
		storedRelease(t, 107, "1.14.1", coreartifact.ArchitectureAMD64, coreartifact.ArchitectureARM64),
	}}
	validator, err := encodeTagValidator([]string{`W/"same"`})
	if err != nil {
		t.Fatalf("encodeTagValidator: %v", err)
	}
	// GitHub answers a weak tag with its strong form when the encoding differs.
	notModified := make(http.Header)
	notModified.Set("ETag", `"same"`)
	doer := &queueDoer{responses: []*http.Response{
		{StatusCode: http.StatusNotModified, Header: notModified, Body: io.NopCloser(strings.NewReader(""))},
	}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err := client.Refresh(context.Background(), validator, previous)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	requests := doer.Requests()
	if !result.NotModified || result.ETag != validator || len(result.Catalog.Releases) != 1 || len(requests) != 1 ||
		requests[0].Header.Get("If-None-Match") != `W/"same"` {
		t.Fatalf("unchanged tags: result = %+v, requests = %d", result, len(requests))
	}

	// A new prerelease tag changes the page but no stable release: the tags
	// are read again, and neither the repository nor a release.
	changed := `[{"name":"v1.15.0-alpha.1"},{"name":"v1.14.1"}]`
	doer = &queueDoer{responses: []*http.Response{
		jsonResponse(http.StatusOK, changed, tagPageHeaders(`W/"changed"`, false)),
		jsonResponse(http.StatusOK, changed, tagPageHeaders(`W/"changed"`, false)),
	}}
	client, err = NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err = client.Refresh(context.Background(), validator, previous)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	pageETags, _ := client.decodeTagValidator(result.ETag)
	if !result.NotModified || len(pageETags) != 1 || pageETags[0] != `W/"changed"` || len(doer.Requests()) != 2 {
		t.Fatalf("prerelease tag: result = %+v, requests = %d", result, len(doer.Requests()))
	}

	// A validator of an earlier refresh names release pages; it is ignored
	// rather than failing every refresh of a panel that stored one.
	for _, earlier := range []string{"sbp-github-pages-v1.e30", "sbp-github-pages-v3.e30"} {
		if pages, err := client.decodeTagValidator(earlier); err != nil || pages != nil {
			t.Fatalf("decodeTagValidator(%q) = (%q, %v), want it ignored", earlier, pages, err)
		}
	}
}

func TestOfficialCatalogKeepsOnlyOneMuslAssetPerArchitecture(t *testing.T) {
	t.Parallel()
	var assets []githubAsset
	for index, platform := range []string{"amd64", "amd64-glibc", "amd64v3", "amd64-musl", "arm64", "arm64-glibc", "arm64-musl"} {
		name := "sing-box-1.14.1-linux-" + platform + ".tar.gz"
		assets = append(assets, githubAsset{
			ID: int64(index + 1), Name: name, Size: 100,
			BrowserDownloadURL: "https://github.com/SagerNet/sing-box/releases/download/v1.14.1/" + name,
		})
	}
	client := &GitHubClient{}
	releases, _, err := client.filter(OfficialRepositoryID, []githubRelease{
		{ID: 101, TagName: "v1.14.1", Assets: assets},
		{ID: 102, TagName: "v1.12.25"},
	})
	if err != nil || len(releases) != 1 || len(releases[0].Assets) != 2 {
		t.Fatalf("musl catalog = %+v, error = %v", releases, err)
	}
	for _, asset := range releases[0].Assets {
		if asset.Variant != coreartifact.VariantMusl {
			t.Fatalf("unexpected build: %+v", asset)
		}
	}
	duplicate := assets[3]
	duplicate.ID = 99
	_, _, err = client.filter(OfficialRepositoryID, []githubRelease{{
		ID: 101, TagName: "v1.14.1", Assets: append(assets, duplicate),
	}})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "duplicate_platform_asset" {
		t.Fatalf("duplicate platform error = %v", err)
	}
}

func TestGitHubRefreshReadsOnlyNewAndIncompleteReleases(t *testing.T) {
	t.Parallel()
	both := []coreartifact.Architecture{coreartifact.ArchitectureAMD64, coreartifact.ArchitectureARM64}
	previous := &Catalog{RepositoryID: OfficialRepositoryID, Releases: []Release{
		storedRelease(t, 107, "1.14.1", both...),
		storedRelease(t, 105, "1.13.20", both...),
		// Its arm64 package was not uploaded when it was read.
		storedRelease(t, 101, "1.13.19", coreartifact.ArchitectureAMD64),
	}}
	// v1.14.3 is tagged before its release is published, and v1.13.20 is gone.
	tags := `[{"name":"v1.14.3"},{"name":"v1.14.2"},{"name":"v1.14.1"},{"name":"v1.13.19"}]`
	doer := &queueDoer{responses: []*http.Response{
		jsonResponse(http.StatusOK, tags, tagPageHeaders(`W/"tags"`, false)),
		jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil),
		jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`, nil),
		jsonResponse(http.StatusOK, githubReleaseJSON(109, "1.14.2", "amd64", "arm64"), nil),
		jsonResponse(http.StatusOK, githubReleaseJSON(101, "1.13.19", "amd64", "arm64"), nil),
	}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err := client.Refresh(context.Background(), "", previous)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	versions := make([]string, 0, len(result.Catalog.Releases))
	for _, release := range result.Catalog.Releases {
		versions = append(versions, fmt.Sprintf("%s:%d", release.Version, len(release.Assets)))
	}
	if result.NotModified || strings.Join(versions, ",") != "1.14.2:2,1.14.1:2,1.13.19:2" {
		t.Fatalf("catalog = %v, not modified = %t", versions, result.NotModified)
	}
	paths := make([]string, 0, len(doer.Requests()))
	for _, request := range doer.Requests() {
		paths = append(paths, request.URL.Path)
	}
	want := "/repos/SagerNet/sing-box/tags,/repos/SagerNet/sing-box,/repos/SagerNet/sing-box/releases/tags/v1.14.3," +
		"/repos/SagerNet/sing-box/releases/tags/v1.14.2,/repos/SagerNet/sing-box/releases/tags/v1.13.19"
	if strings.Join(paths, ",") != want {
		t.Fatalf("requests = %v", paths)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "release_not_published"
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want the unpublished release noted", result.Diagnostics)
	}
}

func TestGitHubRefreshRejectsNotModifiedWithoutCacheValidator(t *testing.T) {
	t.Parallel()
	doer := &queueDoer{responses: []*http.Response{{
		StatusCode: http.StatusNotModified,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	if _, err := client.Refresh(context.Background(), "", nil); err == nil {
		t.Fatalf("Refresh accepted 304 without a prior ETag")
	}
}

func TestGitHubRefreshRejectsDuplicateJSONAndHidesCredentials(t *testing.T) {
	t.Parallel()
	t.Run("duplicate JSON", func(t *testing.T) {
		doer := &queueDoer{responses: []*http.Response{
			jsonResponse(http.StatusOK, `[{"name":"v1.14.0","name":"v1.14.1"}]`, nil),
		}}
		client, err := NewGitHubClient(ClientOptions{HTTP: doer})
		if err != nil {
			t.Fatalf("NewGitHubClient: %v", err)
		}
		if _, err := client.Refresh(context.Background(), "", nil); err == nil {
			t.Fatalf("Refresh accepted duplicate JSON keys")
		}
	})
	t.Run("safe failure text", func(t *testing.T) {
		client, err := NewGitHubClient(ClientOptions{
			HTTP:  doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret-token") }),
			Token: "secret-token",
		})
		if err != nil {
			t.Fatalf("NewGitHubClient: %v", err)
		}
		_, err = client.Refresh(context.Background(), "", nil)
		if err == nil || strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("safe error = %v", err)
		}
	})
}

func TestGitHubRefreshPropagatesCancellation(t *testing.T) {
	t.Parallel()
	client, err := NewGitHubClient(ClientOptions{HTTP: doerFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Refresh(ctx, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh cancellation error = %v, want context.Canceled", err)
	}
}

func TestGitHubRefreshSupportsMoreThanThirtyTagPages(t *testing.T) {
	t.Parallel()

	responses := make([]*http.Response, 0, 32)
	for page := 1; page <= 31; page++ {
		body := fmt.Sprintf(`[{"name":"v1.14.0-alpha.%d"}]`, page)
		responses = append(responses, jsonResponse(http.StatusOK, body, tagPageHeaders(`"page-`+fmt.Sprint(page)+`"`, page < 31)))
	}
	responses = append(responses, jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil))
	doer := &queueDoer{responses: responses}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	if _, err := client.Refresh(context.Background(), "", nil); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if requests := doer.Requests(); len(requests) != 32 {
		t.Fatalf("requests = %d, want 31 tag pages plus repository", len(requests))
	}
}

func TestGitHubRefreshEnforcesAggregateBodyBudget(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Link", `<https://api.github.com/next>; rel="next"`)
	tag := `{"name":"v1.14.0-alpha.1"}`
	body := "[" + tag + strings.Repeat(" ", 76-len(tag)) + "]"
	doer := &queueDoer{responses: []*http.Response{
		jsonResponse(http.StatusOK, body, headers),
		jsonResponse(http.StatusOK, body, headers),
	}}
	client, err := NewGitHubClient(ClientOptions{
		HTTP: doer, MaximumBytesPerPage: 100, MaximumTotalBytes: 150,
	})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	_, err = client.Refresh(context.Background(), "", nil)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "total_body_too_large" {
		t.Fatalf("Refresh error = %v, want total_body_too_large", err)
	}
}

func TestCatalogFailuresNameTheirCauseForThePanelLog(t *testing.T) {
	t.Parallel()
	refresh := func(doer HTTPDoer) error {
		client, err := NewGitHubClient(ClientOptions{HTTP: doer})
		if err != nil {
			t.Fatalf("NewGitHubClient: %v", err)
		}
		_, err = client.Refresh(context.Background(), "", nil)
		return err
	}
	tests := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{
			name:    "GitHub status",
			err:     refresh(&queueDoer{responses: []*http.Response{jsonResponse(http.StatusBadGateway, `{}`, nil)}}),
			code:    "catalog_http_status",
			message: "catalog tags failed (http_status, HTTP 502)",
		},
		{
			name: "timeout",
			err: refresh(doerFunc(func(*http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("read: %w", context.DeadlineExceeded)
			})),
			code:    "catalog_timeout",
			message: "catalog tags failed (timeout)",
		},
		{
			name:    "download status",
			err:     &DownloadError{Code: "http_status", Status: http.StatusNotFound},
			code:    "download_http_status",
			message: "download artifact: http_status (HTTP 404)",
		},
	}
	for _, test := range tests {
		code, message, ok := ClassifyOperationFailure(test.err)
		if !ok || code != test.code || message != test.message {
			t.Fatalf("%s: ClassifyOperationFailure(%v) = (%q, %q, %t), want (%q, %q)", test.name, test.err, code, message, ok, test.code, test.message)
		}
	}
	if _, _, ok := ClassifyOperationFailure(errors.New("https://example.test/?token=secret")); ok {
		t.Fatal("an unknown error was classified")
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (doer doerFunc) Do(request *http.Request) (*http.Response, error) { return doer(request) }

type queueDoer struct {
	responses []*http.Response
	requests  []*http.Request
}

func (doer *queueDoer) Do(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	doer.requests = append(doer.requests, clone)
	if len(doer.responses) == 0 {
		return nil, errors.New("unexpected request")
	}
	response := doer.responses[0]
	doer.responses = doer.responses[1:]
	response.Request = request
	return response, nil
}

func (doer *queueDoer) Requests() []*http.Request {
	return append([]*http.Request(nil), doer.requests...)
}

func jsonResponse(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func digest(t *testing.T, pair string) coreartifact.SHA256 {
	t.Helper()
	parsed, err := coreartifact.ParseSHA256(strings.Repeat(pair, 32))
	if err != nil {
		t.Fatalf("ParseSHA256: %v", err)
	}
	return parsed
}

func exactVersion(t *testing.T, value string) coreartifact.ExactVersion {
	t.Helper()
	parsed, err := coreartifact.ParseExactVersion(value)
	if err != nil {
		t.Fatalf("ParseExactVersion: %v", err)
	}
	return parsed
}

// tagPageHeaders gives a tag page its ETag and, when more pages follow, a
// link to the next.
func tagPageHeaders(etag string, next bool) http.Header {
	headers := make(http.Header)
	headers.Set("ETag", etag)
	if next {
		headers.Set("Link", `<https://api.github.com/next>; rel="next"`)
	}
	return headers
}

// storedRelease is a catalog release of version with a musl package for each
// architecture.
func storedRelease(t *testing.T, id int64, version string, architectures ...coreartifact.Architecture) Release {
	t.Helper()
	release := Release{ID: id, Tag: "v" + version, Version: exactVersion(t, version)}
	for index, architecture := range architectures {
		name := "sing-box-" + version + "-linux-" + string(architecture) + "-musl.tar.gz"
		release.Assets = append(release.Assets, Asset{
			RepositoryID: OfficialRepositoryID, ReleaseID: id, AssetID: id*10 + int64(index), Name: name,
			DownloadURL: "https://github.com/SagerNet/sing-box/releases/download/v" + version + "/" + name,
			Size:        100, Version: release.Version, OperatingSystem: coreartifact.OperatingSystemLinux,
			Architecture: architecture, Variant: coreartifact.VariantMusl,
		})
	}
	return release
}

// githubReleaseJSON is GitHub's release of version with a musl package for
// each architecture.
func githubReleaseJSON(id int64, version string, architectures ...string) string {
	assets := make([]string, 0, len(architectures))
	for index, architecture := range architectures {
		name := "sing-box-" + version + "-linux-" + architecture + "-musl.tar.gz"
		assets = append(assets, fmt.Sprintf(`{"id":%d,"name":%q,"size":100,"browser_download_url":%q}`,
			id*10+int64(index), name, "https://github.com/SagerNet/sing-box/releases/download/v"+version+"/"+name))
	}
	return fmt.Sprintf(`{"id":%d,"tag_name":"v%s","draft":false,"prerelease":false,"assets":[%s]}`, id, version, strings.Join(assets, ","))
}
