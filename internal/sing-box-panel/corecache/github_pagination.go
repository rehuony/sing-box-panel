// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

type githubTag struct {
	Name string `json:"name"`
}

// loadTagPages reads the repository's tags, which GitHub lists newest
// version first, until a page holds no tag of a supported line: the pages
// after it hold only older tags. It returns the tags and each page's ETag.
func (client *GitHubClient) loadTagPages(ctx context.Context, remainingBytes *int64) ([]string, []string, error) {
	tags := make([]string, 0)
	pageETags := make([]string, 0)
	for page := 1; page <= client.maximumPages; page++ {
		response, body, err := client.requestJSON(ctx, StepTags, tagPageURL(page), nil, false, remainingBytes)
		if err != nil {
			return nil, nil, err
		}
		pageETag := response.Header.Get("ETag")
		if pageETag != "" && !validHTTPETag(pageETag) {
			return nil, nil, fail(StepTags, "invalid_page_etag", nil)
		}
		pageETags = append(pageETags, pageETag)
		var pageTags []githubTag
		if err := decodeGitHubJSON(body, &pageTags); err != nil {
			return nil, nil, fail(StepTags, "invalid_json", err)
		}
		for _, tag := range pageTags {
			tags = append(tags, tag.Name)
		}
		if !hasNextLink(response.Header.Get("Link")) || !slices.ContainsFunc(pageTags, func(tag githubTag) bool { return supportedTag(tag.Name) }) {
			return tags, pageETags, nil
		}
	}
	return nil, nil, fail(StepTags, "page_limit", nil)
}

// tagPagesUnchanged asks for each tag page read last time with its ETag. A
// new or removed tag changes the page holding it, so unchanged pages up to
// the one that ended discovery leave the catalog unchanged.
func (client *GitHubClient) tagPagesUnchanged(ctx context.Context, pageETags []string, remainingBytes *int64) (bool, error) {
	for index, pageETag := range pageETags {
		headers := make(http.Header)
		headers.Set("If-None-Match", pageETag)
		response, _, err := client.requestJSON(ctx, StepTags, tagPageURL(index+1), headers, true, remainingBytes)
		if err != nil {
			return false, err
		}
		if response.StatusCode != http.StatusNotModified {
			return false, nil
		}
		// GitHub marks the same entity tag weak or strong by the response's
		// encoding, so the two match as If-None-Match does, ignoring W/.
		responseETag := response.Header.Get("ETag")
		if responseETag != "" && (!validHTTPETag(responseETag) || strings.TrimPrefix(responseETag, "W/") != strings.TrimPrefix(pageETag, "W/")) {
			return false, fail(StepTags, "invalid_page_etag", nil)
		}
	}
	return true, nil
}

// loadRelease reads the release of one tag; found is false while the tag
// has no published release, as before its release is published.
func (client *GitHubClient) loadRelease(ctx context.Context, tag string, remainingBytes *int64) (githubRelease, bool, error) {
	_, body, err := client.requestJSON(ctx, StepReleases, releaseByTagURL(tag), nil, false, remainingBytes)
	var failure *Failure
	if errors.As(err, &failure) && failure.Status == http.StatusNotFound {
		return githubRelease{}, false, nil
	}
	if err != nil {
		return githubRelease{}, false, err
	}
	var release githubRelease
	if err := decodeGitHubJSON(body, &release); err != nil {
		return githubRelease{}, false, fail(StepReleases, "invalid_json", err)
	}
	if release.TagName != tag {
		return githubRelease{}, false, fail(StepReleases, "release_tag_mismatch", nil)
	}
	return release, true, nil
}

func (client *GitHubClient) loadRepository(ctx context.Context, remainingBytes *int64) (int64, error) {
	_, repositoryBody, err := client.requestJSON(
		ctx,
		StepRepository,
		githubAPIOrigin+githubRepositoryPath,
		nil,
		false,
		remainingBytes,
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

// refreshPlan is what a refresh keeps of the previous catalog and the tags
// whose releases it reads.
type refreshPlan struct {
	keep    []Release
	read    []string
	dropped int
}

// planRefresh keeps each previous release whose stable tag of a supported
// line is still listed and which holds both architectures' packages, reads
// the release of every other such tag, and drops the previous releases whose
// tags are gone.
func planRefresh(tags []string, previous *Catalog) refreshPlan {
	stored := make(map[coreartifact.ExactVersion]Release)
	if previous != nil {
		for _, release := range previous.Releases {
			stored[release.Version] = release
		}
	}
	plan := refreshPlan{keep: make([]Release, 0), read: make([]string, 0)}
	listed := make(map[coreartifact.ExactVersion]struct{})
	for _, tag := range tags {
		version, stable := stableVersion(tag)
		if !stable || !supportedTag(tag) {
			continue
		}
		if _, duplicate := listed[version]; duplicate {
			continue
		}
		listed[version] = struct{}{}
		if release, ok := stored[version]; ok && completeRelease(release) {
			plan.keep = append(plan.keep, release)
		} else {
			plan.read = append(plan.read, tag)
		}
	}
	for version := range stored {
		if _, ok := listed[version]; !ok {
			plan.dropped++
		}
	}
	return plan
}

// completeRelease reports whether a release holds the packages of both
// supported architectures, after which its release is not read again.
func completeRelease(release Release) bool {
	architectures := make(map[coreartifact.Architecture]struct{}, 2)
	for _, asset := range release.Assets {
		architectures[asset.Architecture] = struct{}{}
	}
	_, amd64 := architectures[coreartifact.ArchitectureAMD64]
	_, arm64 := architectures[coreartifact.ArchitectureARM64]
	return amd64 && arm64
}

func tagPageURL(page int) string {
	return githubAPIOrigin + githubRepositoryPath + "/tags?per_page=" +
		strconv.Itoa(tagsPerPage) + "&page=" + strconv.Itoa(page)
}

func releaseByTagURL(tag string) string {
	return githubAPIOrigin + githubRepositoryPath + "/releases/tags/" + url.PathEscape(tag)
}

type tagValidatorWire struct {
	PageETags []string `json:"page_etags"`
}

func encodeTagValidator(pageETags []string) (string, error) {
	if len(pageETags) == 0 {
		return "", nil
	}
	for _, pageETag := range pageETags {
		// Without every page's ETag the next refresh cannot prove the tags
		// unchanged; it reads them again.
		if pageETag == "" {
			return "", nil
		}
		if !validHTTPETag(pageETag) {
			return "", fail(StepTags, "invalid_page_etag", nil)
		}
	}
	encoded, err := json.Marshal(tagValidatorWire{PageETags: pageETags})
	if err != nil {
		return "", fail(StepTags, "validator_encode", err)
	}
	value := tagValidatorPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if len(value) > maximumValidatorSize {
		return "", fail(StepTags, "validator_too_large", nil)
	}
	return value, nil
}

func (client *GitHubClient) decodeTagValidator(value string) ([]string, error) {
	// Validators of earlier refreshes name release pages; the next refresh
	// reads the tags anew.
	if value == "" || strings.HasPrefix(value, "sbp-github-pages-v") {
		return nil, nil
	}
	if len(value) > maximumValidatorSize || containsControl(value) || !strings.HasPrefix(value, tagValidatorPrefix) {
		return nil, fail(StepTags, "invalid_etag", nil)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, tagValidatorPrefix))
	if err != nil || len(raw) > maximumValidatorSize || rejectDuplicateKeys(raw) != nil {
		return nil, fail(StepTags, "invalid_etag", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire tagValidatorWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fail(StepTags, "invalid_etag", err)
	}
	if err := ensureJSONEOF(decoder); err != nil || len(wire.PageETags) == 0 || len(wire.PageETags) > client.maximumPages {
		return nil, fail(StepTags, "invalid_etag", err)
	}
	for _, pageETag := range wire.PageETags {
		if !validHTTPETag(pageETag) {
			return nil, fail(StepTags, "invalid_etag", nil)
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
