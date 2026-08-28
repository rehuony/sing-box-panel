// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	// OfficialRepositoryID pins the immutable GitHub identity in addition to
	// the human-readable repository path, preventing a name-transfer takeover.
	OfficialRepositoryID int64 = 509091576
	githubAPIOrigin            = "https://api.github.com"
	githubRepositoryPath       = "/repos/SagerNet/sing-box"
	defaultPerPage             = 20
	defaultMaximumPages        = 100
	defaultMaximumPage         = 8 << 20
	defaultMaximumTotal        = 128 << 20
	defaultTimeout             = 3 * time.Minute
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
	MaximumTotalBytes   int64
}

type GitHubClient struct {
	http         HTTPDoer
	digestLookup DigestLookup
	token        string
	timeout      time.Duration
	maximumPages int
	maximumBytes int64
	maximumTotal int64
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
	if options.MaximumTotalBytes <= 0 {
		options.MaximumTotalBytes = defaultMaximumTotal
	}
	if options.MaximumPages > 100 || options.MaximumBytesPerPage > 32<<20 ||
		options.MaximumTotalBytes > 512<<20 || options.MaximumTotalBytes < options.MaximumBytesPerPage ||
		options.Timeout > 5*time.Minute {
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
		maximumTotal: options.MaximumTotalBytes,
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
	remainingBytes := client.maximumTotal

	result := RefreshResult{Diagnostics: make([]Diagnostic, 0)}
	previousPageETags, err := client.decodePageValidator(previousETag)
	if err != nil {
		return RefreshResult{}, err
	}
	if len(previousPageETags) > 0 {
		unchanged, probeErr := client.releasePagesUnchanged(operationContext, previousPageETags, &remainingBytes)
		if probeErr != nil {
			return RefreshResult{}, probeErr
		}
		// DigestLookup is a separate source whose state is not covered by GitHub
		// validators. Without its own immutable snapshot token, refresh it rather
		// than speculating that the combined catalog is unchanged.
		if unchanged && client.digestLookup == nil {
			repositoryID, repositoryErr := client.loadRepository(operationContext, &remainingBytes)
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

	rawReleases, pageETags, diagnostics, err := client.loadReleasePages(operationContext, &remainingBytes)
	if err != nil {
		return RefreshResult{}, err
	}
	result.ETag, err = encodePageValidator(pageETags)
	if err != nil {
		return RefreshResult{}, err
	}
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	repositoryID, err := client.loadRepository(operationContext, &remainingBytes)
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
