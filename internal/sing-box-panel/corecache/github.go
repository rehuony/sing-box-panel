// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

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
	tagsPerPage                = 100
	defaultMaximumPages        = 100
	defaultMaximumPage         = 32 << 20
	defaultMaximumTotal        = 256 << 20
	defaultTimeout             = 3 * time.Minute
	tagValidatorPrefix         = "sbp-github-tags-v1."
	maximumValidatorSize       = 16 << 10
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type ClientOptions struct {
	HTTP                HTTPDoer
	Token               string
	Timeout             time.Duration
	MaximumPages        int
	MaximumBytesPerPage int64
	MaximumTotalBytes   int64
}

type GitHubClient struct {
	http         HTTPDoer
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
		token:        options.Token,
		timeout:      options.Timeout,
		maximumPages: options.MaximumPages,
		maximumBytes: options.MaximumBytesPerPage,
		maximumTotal: options.MaximumTotalBytes,
	}, nil
}

// Refresh brings previous, the stored catalog, up to date with the official
// repository. It reads the tags, which unlike the release list carry no
// download counts and so keep their HTTP validators while no tag changes; when
// every tag page read last time is unchanged it reads nothing more. Otherwise
// it reads a release only for a stable tag of a supported line that previous
// lacks or holds without both architectures' packages, and drops the releases
// whose tags are gone. Without previous it reads every such release.
func (client *GitHubClient) Refresh(ctx context.Context, previousValidator string, previous *Catalog) (RefreshResult, error) {
	if client == nil || client.http == nil {
		return RefreshResult{}, fail(StepTags, "client_unavailable", nil)
	}
	if ctx == nil {
		return RefreshResult{}, fail(StepTags, "nil_context", nil)
	}
	operationContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	remainingBytes := client.maximumTotal
	result := RefreshResult{Diagnostics: make([]Diagnostic, 0)}

	if previous != nil {
		previousPageETags, err := client.decodeTagValidator(previousValidator)
		if err != nil {
			return RefreshResult{}, err
		}
		if len(previousPageETags) > 0 {
			unchanged, err := client.tagPagesUnchanged(operationContext, previousPageETags, &remainingBytes)
			if err != nil {
				return RefreshResult{}, err
			}
			if unchanged {
				result.Catalog, result.ETag, result.NotModified = *previous, previousValidator, true
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Step: StepTags, Severity: DiagnosticInfo, Code: "not_modified", Message: "every official tag page is unchanged"})
				return result, nil
			}
		}
	}

	tags, pageETags, err := client.loadTagPages(operationContext, &remainingBytes)
	if err != nil {
		return RefreshResult{}, err
	}
	if result.ETag, err = encodeTagValidator(pageETags); err != nil {
		return RefreshResult{}, err
	}
	plan := planRefresh(tags, previous)
	if previous != nil && len(plan.read) == 0 && plan.dropped == 0 {
		result.Catalog, result.NotModified = *previous, true
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Step: StepTags, Severity: DiagnosticInfo, Code: "not_modified", Message: "no stable release of a supported line changed"})
		return result, nil
	}

	// A changed catalog comes from the repository the pinned identity names.
	repositoryID, err := client.loadRepository(operationContext, &remainingBytes)
	if err != nil {
		return RefreshResult{}, err
	}
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Step: StepRepository, Severity: DiagnosticInfo, Code: "identity_verified", Message: "official repository identity was resolved"})
	rawReleases := make([]githubRelease, 0, len(plan.read))
	for _, tag := range plan.read {
		release, found, err := client.loadRelease(operationContext, tag, &remainingBytes)
		if err != nil {
			return RefreshResult{}, err
		}
		if !found {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Step: StepReleases, Severity: DiagnosticInfo, Code: "release_not_published", Message: "a stable tag has no published release yet"})
			continue
		}
		rawReleases = append(rawReleases, release)
	}
	filtered, diagnostics, err := client.filter(repositoryID, rawReleases)
	if err != nil {
		return RefreshResult{}, err
	}
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	result.Catalog = Catalog{RepositoryID: repositoryID, Releases: append(plan.keep, filtered...)}
	sortCatalog(&result.Catalog)
	return result, nil
}
