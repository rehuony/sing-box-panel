// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

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

func TestGitHubRefreshFiltersPaginatesAndResolvesDigests(t *testing.T) {
	t.Parallel()
	apiDigest := digest(t, "11")
	pageOne := `[
		{"id":101,"tag_name":"v1.13.19","draft":false,"prerelease":false,"unknown":"accepted","assets":[
		{"id":1001,"name":"sing-box-1.13.19-linux-amd64-musl.tar.gz","size":100,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-musl.tar.gz","digest":"sha256:` + apiDigest.String() + `"},
		{"id":1002,"name":"sing-box-1.13.19-linux-arm64-musl.tar.gz","size":101,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-arm64-musl.tar.gz","digest":"invalid-upstream-digest"},
		{"id":1003,"name":"sing-box-1.13.19-linux-amd64-glibc.tar.gz","size":102,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-glibc.tar.gz","digest":"sha256:` + apiDigest.String() + `"},
		{"id":1004,"name":"sing-box-1.13.19-windows-amd64.zip","size":103,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-windows-amd64.zip","digest":"sha256:` + apiDigest.String() + `"}
      ]},
      {"id":102,"tag_name":"v1.14.0-beta.1","draft":false,"prerelease":true,"assets":[]}
    ]`
	pageTwo := `[
      {"id":103,"tag_name":"v1.13.18","draft":false,"prerelease":false,"assets":[{"id":1005,"name":"sing-box-1.13.18-linux-amd64-musl.tar.gz","size":102,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.18/sing-box-1.13.18-linux-amd64-musl.tar.gz","digest":"sha256:` + apiDigest.String() + `"}]},
      {"id":106,"tag_name":"v1.12.3","draft":false,"prerelease":false,"assets":[]},
      {"id":104,"tag_name":"latest","draft":false,"prerelease":false,"assets":[]},
      {"id":105,"tag_name":"v1.11.0","draft":true,"prerelease":false,"assets":[]}
    ]`
	pageHeaders := make(http.Header)
	pageHeaders.Set("ETag", `W/"new-page-1"`)
	pageHeaders.Set("Link", `<https://api.github.com/next>; rel="next"`)
	pageTwoHeaders := make(http.Header)
	pageTwoHeaders.Set("ETag", `W/"new-page-2"`)
	doer := &queueDoer{responses: []*http.Response{
		jsonResponse(http.StatusOK, pageOne, pageHeaders),
		jsonResponse(http.StatusOK, pageTwo, pageTwoHeaders),
		jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil),
	}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer, Token: "secret-token"})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err := client.Refresh(context.Background(), "")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.NotModified || result.ETag == "" || result.Catalog.RepositoryID != OfficialRepositoryID {
		t.Fatalf("refresh metadata = %+v", result)
	}
	pageETags, err := client.decodePageValidator(result.ETag)
	if err != nil || len(pageETags) != 2 || pageETags[0] != `W/"new-page-1"` || pageETags[1] != `W/"new-page-2"` {
		t.Fatalf("page validator = (%q, %v)", pageETags, err)
	}
	if len(result.Catalog.Releases) != 2 || result.Catalog.Releases[0].Version.String() != "1.13.19" || result.Catalog.Releases[1].Version.String() != "1.13.18" {
		t.Fatalf("filtered releases = %+v", result.Catalog.Releases)
	}
	assets := result.Catalog.Releases[0].Assets
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
	if len(requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(requests))
	}
	if got := requests[0].Header.Get("If-None-Match"); got != "" {
		t.Fatalf("first request If-None-Match = %q, want empty", got)
	}
	if got := requests[1].Header.Get("If-None-Match"); got != "" {
		t.Fatalf("second request If-None-Match = %q, want empty", got)
	}
	if got := requests[0].Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestGitHubRefreshNotModifiedAvoidsOtherRequests(t *testing.T) {
	t.Parallel()
	previous, err := encodePageValidator([]string{`W/"same"`})
	if err != nil {
		t.Fatalf("encodePageValidator: %v", err)
	}
	headers := make(http.Header)
	headers.Set("ETag", `W/"same"`)
	doer := &queueDoer{responses: []*http.Response{
		{
			StatusCode: http.StatusNotModified,
			Header:     headers,
			Body:       io.NopCloser(strings.NewReader("")),
		},
		jsonResponse(http.StatusOK, `[]`, nil),
		jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil),
	}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err := client.Refresh(context.Background(), previous)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !result.NotModified || result.ETag != previous || result.Catalog.RepositoryID != OfficialRepositoryID || len(doer.Requests()) != 3 {
		t.Fatalf("not-modified result = %+v, requests = %d", result, len(doer.Requests()))
	}
	requests := doer.Requests()
	if requests[0].Header.Get("If-None-Match") != `W/"same"` || requests[1].Header.Get("If-None-Match") != "" {
		t.Fatalf("conditional page requests were not scoped correctly")
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

func TestGitHubRefreshDetectsChangeOutsideFirstPage(t *testing.T) {
	t.Parallel()
	previous, err := encodePageValidator([]string{`W/"page-1"`, `W/"old-page-2"`})
	if err != nil {
		t.Fatalf("encodePageValidator: %v", err)
	}
	pageOneNotModifiedHeaders := make(http.Header)
	pageOneNotModifiedHeaders.Set("ETag", `W/"page-1"`)
	pageOneFullHeaders := make(http.Header)
	pageOneFullHeaders.Set("ETag", `W/"page-1"`)
	pageOneFullHeaders.Set("Link", `<https://api.github.com/next>; rel="next"`)
	pageTwoHeaders := make(http.Header)
	pageTwoHeaders.Set("ETag", `W/"new-page-2"`)
	changedPage := `[{"id":301,"tag_name":"v1.2.3","draft":false,"prerelease":false,"assets":[{"id":1008,"name":"sing-box-1.2.3-linux-arm64-musl.tar.gz","size":100,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.2.3/sing-box-1.2.3-linux-arm64-musl.tar.gz"}]}]`
	doer := &queueDoer{responses: []*http.Response{
		{StatusCode: http.StatusNotModified, Header: pageOneNotModifiedHeaders, Body: io.NopCloser(strings.NewReader(""))},
		jsonResponse(http.StatusOK, changedPage, pageTwoHeaders),
		jsonResponse(http.StatusOK, `[]`, pageOneFullHeaders),
		jsonResponse(http.StatusOK, changedPage, pageTwoHeaders),
		jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil),
	}}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	result, err := client.Refresh(context.Background(), previous)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.NotModified || len(result.Catalog.Releases) != 1 || result.Catalog.Releases[0].Version.String() != "1.2.3" {
		t.Fatalf("second-page change was not loaded: %+v", result)
	}
	requests := doer.Requests()
	if len(requests) != 5 || requests[0].Header.Get("If-None-Match") != `W/"page-1"` ||
		requests[1].Header.Get("If-None-Match") != `W/"old-page-2"` || requests[2].Header.Get("If-None-Match") != "" {
		t.Fatalf("unexpected conditional/full request sequence")
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
	if _, err := client.Refresh(context.Background(), ""); err == nil {
		t.Fatalf("Refresh accepted 304 without a prior ETag")
	}
}

func TestGitHubRefreshRejectsDuplicateJSONAndHidesCredentials(t *testing.T) {
	t.Parallel()
	t.Run("duplicate JSON", func(t *testing.T) {
		doer := &queueDoer{responses: []*http.Response{
			jsonResponse(http.StatusOK, `[{"id":1,"id":2,"tag_name":"v1.0.0","draft":false,"prerelease":false,"assets":[]}]`, nil),
		}}
		client, err := NewGitHubClient(ClientOptions{HTTP: doer})
		if err != nil {
			t.Fatalf("NewGitHubClient: %v", err)
		}
		if _, err := client.Refresh(context.Background(), ""); err == nil {
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
		_, err = client.Refresh(context.Background(), "")
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
	_, err = client.Refresh(ctx, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh cancellation error = %v, want context.Canceled", err)
	}
}

func TestGitHubRefreshSupportsMoreThanThirtyReleasePages(t *testing.T) {
	t.Parallel()

	responses := make([]*http.Response, 0, 32)
	for page := 1; page <= 31; page++ {
		headers := make(http.Header)
		headers.Set("ETag", `"page-`+fmt.Sprint(page)+`"`)
		if page < 31 {
			headers.Set("Link", `<https://api.github.com/next>; rel="next"`)
		}
		responses = append(responses, jsonResponse(http.StatusOK, `[]`, headers))
	}
	responses = append(responses, jsonResponse(http.StatusOK, `{"id":509091576,"full_name":"SagerNet/sing-box"}`, nil))
	doer := &queueDoer{responses: responses}
	client, err := NewGitHubClient(ClientOptions{HTTP: doer})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	if _, err := client.Refresh(context.Background(), ""); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	requests := doer.Requests()
	if len(requests) != 32 {
		t.Fatalf("requests = %d, want 31 release pages plus repository", len(requests))
	}
	if !strings.Contains(requests[0].URL.RawQuery, "per_page=50") {
		t.Fatalf("release query = %q, want per_page=50", requests[0].URL.RawQuery)
	}
}

func TestGitHubRefreshEnforcesAggregateBodyBudget(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Link", `<https://api.github.com/next>; rel="next"`)
	body := "[" + strings.Repeat(" ", 76) + "]"
	doer := &queueDoer{responses: []*http.Response{
		jsonResponse(http.StatusOK, body, headers),
		jsonResponse(http.StatusOK, body, nil),
	}}
	client, err := NewGitHubClient(ClientOptions{
		HTTP: doer, MaximumBytesPerPage: 100, MaximumTotalBytes: 150,
	})
	if err != nil {
		t.Fatalf("NewGitHubClient: %v", err)
	}
	_, err = client.Refresh(context.Background(), "")
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "total_body_too_large" {
		t.Fatalf("Refresh error = %v, want total_body_too_large", err)
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
