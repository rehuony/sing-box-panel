// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestAssetTrustedDigest(t *testing.T) {
	t.Parallel()
	first := digest(t, "11")
	second := digest(t, "22")
	tests := []struct {
		name    string
		asset   Asset
		want    coreartifact.SHA256
		wantErr error
	}{
		{name: "API only", asset: Asset{APIDigest: first, HasAPIDigest: true}, want: first},
		{name: "catalog only", asset: Asset{CatalogDigest: first, HasCatalogDigest: true}, want: first},
		{name: "both agree", asset: Asset{APIDigest: first, HasAPIDigest: true, CatalogDigest: first, HasCatalogDigest: true}, want: first},
		{name: "missing", asset: Asset{}, wantErr: ErrDigestMissing},
		{name: "mismatch", asset: Asset{APIDigest: first, HasAPIDigest: true, CatalogDigest: second, HasCatalogDigest: true}, wantErr: ErrDigestMismatch},
		{name: "present zero", asset: Asset{HasAPIDigest: true}, wantErr: ErrInvalidCandidate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := test.asset.TrustedDigest()
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("TrustedDigest() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("TrustedDigest() = (%s, %v), want (%s, nil)", got, err, test.want)
			}
		})
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
		{name: "sing-box-1.13.19-linux-amd64.tar.gz", arch: coreartifact.ArchitectureAMD64, variant: coreartifact.VariantPlain, valid: true},
		{name: "sing-box-1.13.19-linux-amd64-glibc.tar.gz", arch: coreartifact.ArchitectureAMD64, variant: coreartifact.VariantGlibc, valid: true},
		{name: "sing-box-1.13.19-linux-amd64v3.tar.gz", arch: coreartifact.ArchitectureAMD64, variant: "amd64v3", valid: true},
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
	catalogDigest := digest(t, "22")
	pageOne := `[
		{"id":101,"tag_name":"v1.13.19","draft":false,"prerelease":false,"unknown":"accepted","assets":[
		{"id":1001,"name":"sing-box-1.13.19-linux-amd64.tar.gz","size":100,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz","digest":"sha256:` + apiDigest.String() + `"},
		{"id":1002,"name":"sing-box-1.13.19-linux-arm64.tar.gz","size":101,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-arm64.tar.gz","digest":""},
		{"id":1003,"name":"sing-box-1.13.19-linux-amd64-glibc.tar.gz","size":102,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64-glibc.tar.gz","digest":"sha256:` + apiDigest.String() + `"},
		{"id":1004,"name":"sing-box-1.13.19-windows-amd64.zip","size":103,"browser_download_url":"https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-windows-amd64.zip","digest":"sha256:` + apiDigest.String() + `"}
      ]},
      {"id":102,"tag_name":"v1.14.0-beta.1","draft":false,"prerelease":true,"assets":[]}
    ]`
	pageTwo := `[
      {"id":103,"tag_name":"v1.12.3","draft":false,"prerelease":false,"assets":[]},
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
	lookup := digestLookupFunc(func(_ context.Context, key DigestKey) (coreartifact.SHA256, bool, error) {
		switch key.AssetID {
		case 1001:
			return apiDigest, true, nil
		case 1002:
			return catalogDigest, true, nil
		case 1003:
			return catalogDigest, true, nil
		default:
			return coreartifact.SHA256{}, false, nil
		}
	})
	client, err := NewGitHubClient(ClientOptions{HTTP: doer, DigestLookup: lookup, Token: "secret-token"})
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
	if len(result.Catalog.Releases) != 2 || result.Catalog.Releases[0].Version.String() != "1.13.19" || result.Catalog.Releases[1].Version.String() != "1.12.3" {
		t.Fatalf("filtered releases = %+v", result.Catalog.Releases)
	}
	assets := result.Catalog.Releases[0].Assets
	if len(assets) != 3 {
		t.Fatalf("stable Linux assets = %d, want 3", len(assets))
	}
	assetsByID := make(map[int64]Asset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.AssetID] = asset
	}
	if trusted, err := assetsByID[1001].TrustedDigest(); err != nil || trusted != apiDigest {
		t.Fatalf("amd64 trusted digest = (%s, %v), want API digest", trusted, err)
	}
	if trusted, err := assetsByID[1002].TrustedDigest(); err != nil || trusted != catalogDigest {
		t.Fatalf("arm64 trusted digest = (%s, %v), want catalog digest", trusted, err)
	}
	mismatchFound := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "digest_mismatch" {
			mismatchFound = true
		}
		if strings.Contains(diagnostic.Message, "secret-token") {
			t.Fatalf("diagnostic leaked token: %+v", diagnostic)
		}
	}
	if !mismatchFound {
		t.Fatalf("digest mismatch diagnostic missing: %+v", result.Diagnostics)
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
	changedPage := `[{"id":301,"tag_name":"v1.2.3","draft":false,"prerelease":false,"assets":[]}]`
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

type digestLookupFunc func(context.Context, DigestKey) (coreartifact.SHA256, bool, error)

func (lookup digestLookupFunc) Lookup(ctx context.Context, key DigestKey) (coreartifact.SHA256, bool, error) {
	return lookup(ctx, key)
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
