// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"context"
	"io"
	"mime"
	"net/http"
)

func (client *GitHubClient) requestJSON(
	ctx context.Context,
	step Step,
	endpoint string,
	headers http.Header,
	allowNotModified bool,
	remainingBytes *int64,
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
	if remainingBytes == nil || int64(len(body)) > *remainingBytes {
		return nil, nil, fail(step, "total_body_too_large", nil)
	}
	*remainingBytes -= int64(len(body))
	return response, body, nil
}
