// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (client *GitHubClient) loadReleasePages(ctx context.Context, remainingBytes *int64) ([]githubRelease, []string, []Diagnostic, error) {
	rawReleases := make([]githubRelease, 0)
	pageETags := make([]string, 0)
	diagnostics := make([]Diagnostic, 0)
	for page := 1; page <= client.maximumPages; page++ {
		response, body, err := client.requestJSON(ctx, StepReleases, releasePageURL(page), nil, false, remainingBytes)
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

func (client *GitHubClient) releasePagesUnchanged(ctx context.Context, pageETags []string, remainingBytes *int64) (bool, error) {
	for index, pageETag := range pageETags {
		headers := make(http.Header)
		headers.Set("If-None-Match", pageETag)
		response, _, err := client.requestJSON(ctx, StepReleases, releasePageURL(index+1), headers, true, remainingBytes)
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
	response, body, err := client.requestJSON(ctx, StepReleases, releasePageURL(len(pageETags)+1), nil, false, remainingBytes)
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
