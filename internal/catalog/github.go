// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

const (
	// OfficialRepositoryID pins the immutable GitHub identity in addition to
	// the human-readable repository path, preventing a name-transfer takeover.
	OfficialRepositoryID int64 = 509091576
	githubAPIOrigin            = "https://api.github.com"
	githubRepositoryPath       = "/repos/SagerNet/sing-box"
	defaultPerPage             = 100
	defaultMaximumPages        = 20
	defaultMaximumPage         = 8 << 20
	defaultTimeout             = 20 * time.Second
	pageValidatorPrefix        = "sbp-github-pages-v1."
	maximumValidatorSize       = 16 << 10
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type ClientOptions struct {
	HTTP                HTTPDoer
	DigestLookup        DigestLookup
	Token               string
	Timeout             time.Duration
	MaximumPages        int
	MaximumBytesPerPage int64
}

type GitHubClient struct {
	http         HTTPDoer
	digestLookup DigestLookup
	token        string
	timeout      time.Duration
	maximumPages int
	maximumBytes int64
}

func NewGitHubClient(options ClientOptions) (*GitHubClient, error) {
	if options.Timeout <= 0 {
		options.Timeout = defaultTimeout
	}
	if options.MaximumPages <= 0 {
		options.MaximumPages = defaultMaximumPages
	}
	if options.MaximumBytesPerPage <= 0 {
		options.MaximumBytesPerPage = defaultMaximumPage
	}
	if options.MaximumPages > 100 || options.MaximumBytesPerPage > 32<<20 || options.Timeout > 2*time.Minute {
		return nil, fail(StepReleases, "invalid_limits", nil)
	}
	if len(options.Token) > 8192 || containsControl(options.Token) {
		return nil, fail(StepReleases, "invalid_token", nil)
	}
	if options.HTTP == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		options.HTTP = &http.Client{
			Transport: transport,
			Timeout:   options.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("catalog redirect rejected")
			},
		}
	}
	return &GitHubClient{
		http:         options.HTTP,
		digestLookup: options.DigestLookup,
		token:        options.Token,
		timeout:      options.Timeout,
		maximumPages: options.MaximumPages,
		maximumBytes: options.MaximumBytesPerPage,
	}, nil
}

func (client *GitHubClient) Refresh(ctx context.Context, previousETag string) (RefreshResult, error) {
	if client == nil || client.http == nil {
		return RefreshResult{}, fail(StepReleases, "client_unavailable", nil)
	}
	if ctx == nil {
		return RefreshResult{}, fail(StepReleases, "nil_context", nil)
	}
	operationContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	result := RefreshResult{Diagnostics: make([]Diagnostic, 0)}
	previousPageETags, err := client.decodePageValidator(previousETag)
	if err != nil {
		return RefreshResult{}, err
	}
	if len(previousPageETags) > 0 {
		unchanged, probeErr := client.releasePagesUnchanged(operationContext, previousPageETags)
		if probeErr != nil {
			return RefreshResult{}, probeErr
		}
		// DigestLookup is a separate source whose state is not covered by GitHub
		// validators. Without its own immutable snapshot token, refresh it rather
		// than speculating that the combined catalog is unchanged.
		if unchanged && client.digestLookup == nil {
			repositoryID, repositoryErr := client.loadRepository(operationContext)
			if repositoryErr != nil {
				return RefreshResult{}, repositoryErr
			}
			result.Catalog.RepositoryID = repositoryID
			result.NotModified = true
			result.ETag = previousETag
			result.Diagnostics = append(result.Diagnostics,
				Diagnostic{Step: StepRepository, Severity: DiagnosticInfo, Code: "identity_verified", Message: "official repository identity was resolved"},
				Diagnostic{Step: StepReleases, Severity: DiagnosticInfo, Code: "not_modified", Message: "every official release page is unchanged"},
			)
			return result, nil
		}
	}

	rawReleases, pageETags, diagnostics, err := client.loadReleasePages(operationContext)
	if err != nil {
		return RefreshResult{}, err
	}
	result.ETag, err = encodePageValidator(pageETags)
	if err != nil {
		return RefreshResult{}, err
	}
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	repositoryID, err := client.loadRepository(operationContext)
	if err != nil {
		return RefreshResult{}, err
	}

	filtered, diagnostics, err := client.filter(operationContext, repositoryID, rawReleases)
	if err != nil {
		return RefreshResult{}, err
	}
	result.Catalog = Catalog{RepositoryID: repositoryID, Releases: filtered}
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Step: StepRepository, Severity: DiagnosticInfo, Code: "identity_verified", Message: "official repository identity was resolved"})
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	sortCatalog(&result.Catalog)
	return result, nil
}

func (client *GitHubClient) loadReleasePages(ctx context.Context) ([]githubRelease, []string, []Diagnostic, error) {
	rawReleases := make([]githubRelease, 0)
	pageETags := make([]string, 0)
	diagnostics := make([]Diagnostic, 0)
	for page := 1; page <= client.maximumPages; page++ {
		response, body, err := client.requestJSON(ctx, StepReleases, releasePageURL(page), nil, false)
		if err != nil {
			return nil, nil, nil, err
		}
		pageETag := response.Header.Get("ETag")
		if pageETag != "" && !validHTTPETag(pageETag) {
			return nil, nil, nil, fail(StepReleases, "invalid_page_etag", nil)
		}
		pageETags = append(pageETags, pageETag)
		var releases []githubRelease
		if err := decodeGitHubJSON(body, &releases); err != nil {
			return nil, nil, nil, fail(StepReleases, "invalid_json", err)
		}
		rawReleases = append(rawReleases, releases...)
		diagnostics = append(diagnostics, Diagnostic{Step: StepReleases, Severity: DiagnosticInfo, Code: "page_loaded", Message: fmt.Sprintf("loaded release page %d", page)})
		if !hasNextLink(response.Header.Get("Link")) {
			return rawReleases, pageETags, diagnostics, nil
		}
		if page == client.maximumPages {
			return nil, nil, nil, fail(StepReleases, "page_limit", nil)
		}
	}
	return nil, nil, nil, fail(StepReleases, "page_limit", nil)
}

func (client *GitHubClient) loadRepository(ctx context.Context) (int64, error) {
	_, repositoryBody, err := client.requestJSON(
		ctx,
		StepRepository,
		githubAPIOrigin+githubRepositoryPath,
		nil,
		false,
	)
	if err != nil {
		return 0, err
	}
	var repository githubRepository
	if err := decodeGitHubJSON(repositoryBody, &repository); err != nil || repository.ID != OfficialRepositoryID || repository.FullName != "SagerNet/sing-box" {
		return 0, fail(StepRepository, "invalid_identity", err)
	}
	return repository.ID, nil
}

func (client *GitHubClient) releasePagesUnchanged(ctx context.Context, pageETags []string) (bool, error) {
	for index, pageETag := range pageETags {
		headers := make(http.Header)
		headers.Set("If-None-Match", pageETag)
		response, _, err := client.requestJSON(ctx, StepReleases, releasePageURL(index+1), headers, true)
		if err != nil {
			return false, err
		}
		if response.StatusCode != http.StatusNotModified {
			return false, nil
		}
		responseETag := response.Header.Get("ETag")
		if (responseETag != "" && !validHTTPETag(responseETag)) || (responseETag != "" && responseETag != pageETag) {
			return false, fail(StepReleases, "invalid_page_etag", nil)
		}
	}
	// Probe the page after the previous end. This detects a newly appended
	// historical page even when every existing page representation is stable.
	response, body, err := client.requestJSON(ctx, StepReleases, releasePageURL(len(pageETags)+1), nil, false)
	if err != nil {
		return false, err
	}
	var releases []githubRelease
	if err := decodeGitHubJSON(body, &releases); err != nil {
		return false, fail(StepReleases, "invalid_json", err)
	}
	return len(releases) == 0 && !hasNextLink(response.Header.Get("Link")), nil
}

func releasePageURL(page int) string {
	return githubAPIOrigin + githubRepositoryPath + "/releases?per_page=" +
		strconv.Itoa(defaultPerPage) + "&page=" + strconv.Itoa(page)
}

type pageValidatorWire struct {
	PageETags []string `json:"page_etags"`
}

func encodePageValidator(pageETags []string) (string, error) {
	if len(pageETags) == 0 {
		return "", nil
	}
	for _, pageETag := range pageETags {
		// A partial validator cannot safely prove a multi-page snapshot. Return
		// no validator and make the next refresh unconditional.
		if pageETag == "" {
			return "", nil
		}
		if !validHTTPETag(pageETag) {
			return "", fail(StepReleases, "invalid_page_etag", nil)
		}
	}
	encoded, err := json.Marshal(pageValidatorWire{PageETags: pageETags})
	if err != nil {
		return "", fail(StepReleases, "validator_encode", err)
	}
	value := pageValidatorPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if len(value) > maximumValidatorSize {
		return "", fail(StepReleases, "validator_too_large", nil)
	}
	return value, nil
}

func (client *GitHubClient) decodePageValidator(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > maximumValidatorSize || containsControl(value) || !strings.HasPrefix(value, pageValidatorPrefix) {
		return nil, fail(StepReleases, "invalid_etag", nil)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, pageValidatorPrefix))
	if err != nil || len(raw) > maximumValidatorSize || rejectDuplicateKeys(raw) != nil {
		return nil, fail(StepReleases, "invalid_etag", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire pageValidatorWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fail(StepReleases, "invalid_etag", err)
	}
	if err := ensureJSONEOF(decoder); err != nil || len(wire.PageETags) == 0 || len(wire.PageETags) > client.maximumPages {
		return nil, fail(StepReleases, "invalid_etag", err)
	}
	for _, pageETag := range wire.PageETags {
		if !validHTTPETag(pageETag) {
			return nil, fail(StepReleases, "invalid_etag", nil)
		}
	}
	return append([]string(nil), wire.PageETags...), nil
}

func validHTTPETag(value string) bool {
	if strings.HasPrefix(value, "W/") {
		value = strings.TrimPrefix(value, "W/")
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for index := 1; index < len(value)-1; index++ {
		character := value[index]
		if character == '"' || character < 0x21 || character == 0x7f {
			return false
		}
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

type githubRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type githubRelease struct {
	ID         int64         `json:"id"`
	TagName    string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func (client *GitHubClient) filter(ctx context.Context, repositoryID int64, releases []githubRelease) ([]Release, []Diagnostic, error) {
	if len(releases) > 5000 {
		return nil, nil, fail(StepFilter, "release_count", nil)
	}
	filtered := make([]Release, 0, len(releases))
	diagnostics := make([]Diagnostic, 0)
	seenVersions := make(map[coreartifact.ExactVersion]struct{})
	seenReleases := make(map[int64]struct{})
	seenAssets := make(map[int64]struct{})
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			diagnostics = append(diagnostics, Diagnostic{Step: StepFilter, Severity: DiagnosticInfo, Code: "unstable_release_skipped", Message: "a draft or prerelease entry was skipped"})
			continue
		}
		version, valid := stableVersion(release.TagName)
		if !valid || release.ID <= 0 {
			diagnostics = append(diagnostics, Diagnostic{Step: StepFilter, Severity: DiagnosticWarning, Code: "invalid_release_skipped", Message: "a release without a strict stable tag or identity was skipped"})
			continue
		}
		if _, duplicate := seenVersions[version]; duplicate {
			return nil, nil, fail(StepFilter, "duplicate_version", nil)
		}
		seenVersions[version] = struct{}{}
		if _, duplicate := seenReleases[release.ID]; duplicate {
			return nil, nil, fail(StepFilter, "duplicate_release_identity", nil)
		}
		seenReleases[release.ID] = struct{}{}
		if len(release.Assets) > 5000 {
			return nil, nil, fail(StepFilter, "asset_count", nil)
		}
		candidateRelease := Release{ID: release.ID, Tag: release.TagName, Version: version, Assets: make([]Asset, 0)}
		for _, rawAsset := range release.Assets {
			if _, duplicate := seenAssets[rawAsset.ID]; duplicate || rawAsset.ID <= 0 {
				return nil, nil, fail(StepFilter, "duplicate_asset_identity", nil)
			}
			seenAssets[rawAsset.ID] = struct{}{}
			architecture, variant, valid := classifyAsset(version, rawAsset.Name)
			if !valid {
				continue
			}
			if rawAsset.Size <= 0 || !validOfficialDownloadURL(rawAsset.BrowserDownloadURL, version, rawAsset.Name) {
				diagnostics = append(diagnostics, Diagnostic{Step: StepFilter, Severity: DiagnosticWarning, Code: "invalid_asset_skipped", Message: "a Linux artifact with invalid size or URL was skipped"})
				continue
			}
			candidate := Asset{
				RepositoryID: repositoryID, ReleaseID: release.ID, AssetID: rawAsset.ID,
				Name: rawAsset.Name, DownloadURL: rawAsset.BrowserDownloadURL, Size: rawAsset.Size,
				Version: version, OperatingSystem: coreartifact.OperatingSystemLinux,
				Architecture: architecture, Variant: variant,
			}
			if rawAsset.Digest != "" {
				apiDigest, err := parseGitHubDigest(rawAsset.Digest)
				if err != nil {
					diagnostics = append(diagnostics, Diagnostic{Step: StepDigest, Severity: DiagnosticWarning, Code: "invalid_api_digest", Message: "an artifact with a malformed API digest was skipped"})
					continue
				}
				candidate.APIDigest, candidate.HasAPIDigest = apiDigest, true
			}
			if client.digestLookup != nil {
				catalogDigest, found, err := client.digestLookup.Lookup(ctx, DigestKey{
					RepositoryID: repositoryID, ReleaseID: release.ID, AssetID: rawAsset.ID,
					Version: version, AssetName: rawAsset.Name,
				})
				if err != nil {
					return nil, nil, fail(StepDigest, "lookup_failed", err)
				}
				if found {
					if catalogDigest.IsZero() {
						return nil, nil, fail(StepDigest, "invalid_catalog_digest", nil)
					}
					candidate.CatalogDigest, candidate.HasCatalogDigest = catalogDigest, true
				}
			}
			if err := candidate.Validate(); err != nil {
				return nil, nil, fail(StepFilter, "invalid_candidate", err)
			}
			if _, err := candidate.TrustedDigest(); err != nil {
				code := "digest_missing"
				if errors.Is(err, ErrDigestMismatch) {
					code = "digest_mismatch"
				}
				diagnostics = append(diagnostics, Diagnostic{Step: StepDigest, Severity: DiagnosticWarning, Code: code, Message: "artifact is visible but official installation is blocked by digest evidence"})
			}
			candidateRelease.Assets = append(candidateRelease.Assets, candidate)
		}
		filtered = append(filtered, candidateRelease)
	}
	return filtered, diagnostics, nil
}

func (client *GitHubClient) requestJSON(
	ctx context.Context,
	step Step,
	endpoint string,
	headers http.Header,
	allowNotModified bool,
) (*http.Response, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fail(step, "request_build", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "sing-box-panel")
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, nil, fail(step, "request_failed", err)
	}
	if response == nil || response.Body == nil {
		return nil, nil, fail(step, "invalid_response", nil)
	}
	defer response.Body.Close()
	if allowNotModified && response.StatusCode == http.StatusNotModified {
		return response, nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, nil, fail(step, "http_status", nil)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, nil, fail(step, "content_type", err)
	}
	if response.ContentLength > client.maximumBytes {
		return nil, nil, fail(step, "body_too_large", nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, client.maximumBytes+1))
	if err != nil {
		return nil, nil, fail(step, "body_read", err)
	}
	if int64(len(body)) > client.maximumBytes {
		return nil, nil, fail(step, "body_too_large", nil)
	}
	return response, body, nil
}

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
