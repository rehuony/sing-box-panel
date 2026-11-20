// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// ErrSourceKindMismatch rejects an operation the source's kind does not
// support: refreshing a file source or giving content to a URL source.
var ErrSourceKindMismatch = errors.New("the subscription source kind does not support the operation")

const minimumSourceRefreshMinutes = 15

// SubscriptionSource is a source as the management API shows it.
type SubscriptionSource struct {
	ID   string                       `json:"id"`
	Name string                       `json:"name"`
	Kind store.SubscriptionSourceKind `json:"kind"`
	URL  string                       `json:"url,omitempty"`
	// RefreshMinutes is present for URL sources only.
	RefreshMinutes *int         `json:"refresh_minutes,omitempty"`
	Format         SourceFormat `json:"format"`
	Filter         SourceFilter `json:"filter"`
	NodeCount      int          `json:"node_count"`
	ContentFormat  SourceFormat `json:"content_format"`
	ContentAt      time.Time    `json:"content_at"`
	LastAttemptAt  *time.Time   `json:"last_attempt_at,omitempty"`
	LastError      string       `json:"last_error,omitempty"`
	NextRefreshAt  *time.Time   `json:"next_refresh_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type SubscriptionSourcePage struct {
	Items []SubscriptionSource `json:"items"`
	Next  *SubscriptionCursor  `json:"next,omitempty"`
}

// CreateSubscriptionSourceRequest creates a URL source, given its URL and
// refresh interval, or a file source, given its content.
type CreateSubscriptionSourceRequest struct {
	Name           string                       `json:"name"`
	Kind           store.SubscriptionSourceKind `json:"kind"`
	URL            *string                      `json:"url"`
	RefreshMinutes *int                         `json:"refresh_minutes"`
	Format         SourceFormat                 `json:"format"`
	Filter         *SourceFilter                `json:"filter"`
	Content        *string                      `json:"content"`
}

// UpdateSubscriptionSourceRequest replaces a source's settings; URL sources
// also give their URL and refresh interval, and a file source may give new
// content.
type UpdateSubscriptionSourceRequest struct {
	Name           string        `json:"name"`
	URL            *string       `json:"url"`
	RefreshMinutes *int          `json:"refresh_minutes"`
	Format         SourceFormat  `json:"format"`
	Filter         *SourceFilter `json:"filter"`
	Content        *string       `json:"content"`
}

// SubscriptionSourcePreviewRequest reads exactly one of a stored source's
// content, a URL's content or the given content.
type SubscriptionSourcePreviewRequest struct {
	SourceID string        `json:"source_id"`
	URL      string        `json:"url"`
	Content  *string       `json:"content"`
	Format   SourceFormat  `json:"format"`
	Filter   *SourceFilter `json:"filter"`
}

type SubscriptionSourcePreviewItem struct {
	Location string `json:"location"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Kept     bool   `json:"kept"`
}

// SubscriptionSourcePreview lists the nodes content holds and whether the
// filter keeps each. Error says why the content cannot be saved.
type SubscriptionSourcePreview struct {
	Format SourceFormat                    `json:"format,omitempty"`
	Items  []SubscriptionSourcePreviewItem `json:"items"`
	Error  *SourceParseError               `json:"error,omitempty"`
}

// sourceSettings are a source's validated settings.
type sourceSettings struct {
	url            string
	refreshMinutes int
	format         SourceFormat
	filter         SourceFilter
}

func validateSourceSettings(kind store.SubscriptionSourceKind, url *string, refreshMinutes *int, format SourceFormat, filter *SourceFilter) (sourceSettings, error) {
	if format == "" {
		format = SourceFormatAuto
	}
	if !ValidSourceFormat(format, true) {
		return sourceSettings{}, &SourceFieldError{Path: "format", Code: "invalid_value"}
	}
	if filter == nil {
		return sourceSettings{}, &SourceFieldError{Path: "filter", Code: "required"}
	}
	if err := filter.Validate(); err != nil {
		return sourceSettings{}, err
	}
	settings := sourceSettings{format: format, filter: SourceFilter{Include: nonNil(filter.Include), Exclude: nonNil(filter.Exclude)}}
	switch kind {
	case store.SubscriptionSourceURL:
		if url == nil {
			return sourceSettings{}, &SourceFieldError{Path: "url", Code: "required"}
		}
		if _, err := parseSourceURL(*url); err != nil || len(*url) > 4096 {
			return sourceSettings{}, &SourceFieldError{Path: "url", Code: "invalid_url"}
		}
		if refreshMinutes == nil {
			return sourceSettings{}, &SourceFieldError{Path: "refresh_minutes", Code: "required"}
		}
		if *refreshMinutes != 0 && (*refreshMinutes < minimumSourceRefreshMinutes || *refreshMinutes > 10080) {
			return sourceSettings{}, &SourceFieldError{Path: "refresh_minutes", Code: "invalid_value"}
		}
		settings.url, settings.refreshMinutes = *url, *refreshMinutes
	case store.SubscriptionSourceFile:
		if url != nil {
			return sourceSettings{}, &SourceFieldError{Path: "url", Code: "unexpected_field"}
		}
		if refreshMinutes != nil {
			return sourceSettings{}, &SourceFieldError{Path: "refresh_minutes", Code: "unexpected_field"}
		}
	default:
		return sourceSettings{}, &SourceFieldError{Path: "kind", Code: "invalid_value"}
	}
	return settings, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string{}, values...)
}

// nextRefresh is when a URL source refreshed at is fetched again, nil when
// it is fetched only on request.
func nextRefresh(minutes int, at time.Time) *time.Time {
	if minutes == 0 {
		return nil
	}
	next := at.Add(time.Duration(minutes) * time.Minute)
	return &next
}

func (application *Service) CreateSubscriptionSource(ctx context.Context, request CreateSubscriptionSourceRequest) (SubscriptionSource, error) {
	settings, err := validateSourceSettings(request.Kind, request.URL, request.RefreshMinutes, request.Format, request.Filter)
	if err != nil {
		return SubscriptionSource{}, err
	}
	var content []byte
	switch request.Kind {
	case store.SubscriptionSourceURL:
		if request.Content != nil {
			return SubscriptionSource{}, &SourceFieldError{Path: "content", Code: "unexpected_field"}
		}
		if content, err = application.fetchSource(ctx, settings.url); err != nil {
			return SubscriptionSource{}, err
		}
	default:
		if request.Content == nil {
			return SubscriptionSource{}, &SourceFieldError{Path: "content", Code: "required"}
		}
		content = []byte(*request.Content)
	}
	reading, err := ReadSource(content, settings.format, settings.filter)
	if err != nil {
		return SubscriptionSource{}, err
	}
	if err := application.ensurePoolCapacity(ctx, "", len(reading.Kept())); err != nil {
		return SubscriptionSource{}, err
	}
	id, err := database.NewID(application.random, "source")
	if err != nil {
		return SubscriptionSource{}, err
	}
	now := application.now().UTC()
	source := store.SubscriptionSource{
		ID: id, Name: request.Name, Kind: request.Kind, URL: settings.url, RefreshMinutes: settings.refreshMinutes,
		Format: string(settings.format), Filter: storeSourceFilter(settings.filter),
		ContentFormat: string(reading.Format), ContentAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if request.Kind == store.SubscriptionSourceURL {
		source.LastAttemptAt, source.NextRefreshAt = &now, nextRefresh(settings.refreshMinutes, now)
	}
	if _, err := application.database.CreateSubscriptionSource(ctx, store.NewSubscriptionSource{Source: source, Content: content}); err != nil {
		return SubscriptionSource{}, err
	}
	application.PoolInputsChanged(ctx)
	return application.SubscriptionSource(ctx, id)
}

func (application *Service) SubscriptionSource(ctx context.Context, sourceID string) (SubscriptionSource, error) {
	stored, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return SubscriptionSource{}, err
	}
	return applicationSubscriptionSource(stored), nil
}

func (application *Service) ListSubscriptionSources(ctx context.Context, request SubscriptionListRequest) (SubscriptionSourcePage, error) {
	stored, err := application.database.ListSubscriptionSources(ctx, store.SubscriptionSourceListFilter{
		Cursor: storeSubscriptionCursor(request.Cursor), Limit: request.Limit,
	})
	if err != nil {
		return SubscriptionSourcePage{}, err
	}
	page := SubscriptionSourcePage{Items: make([]SubscriptionSource, len(stored.Items))}
	for index, source := range stored.Items {
		page.Items[index] = applicationSubscriptionSource(source)
	}
	page.Next = applicationSubscriptionCursor(stored.Next)
	return page, nil
}

// UpdateSubscriptionSource replaces a source's settings, and a file source's
// content when the request gives it, in one write. New content must parse
// with every node the filter keeps converting. Without it, a changed filter
// applies to the stored content at once and must keep only nodes that
// convert, and a changed URL or format applies to the next content.
func (application *Service) UpdateSubscriptionSource(ctx context.Context, sourceID string, request UpdateSubscriptionSourceRequest, expected time.Time) (SubscriptionSource, error) {
	current, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return SubscriptionSource{}, err
	}
	settings, err := validateSourceSettings(current.Kind, request.URL, request.RefreshMinutes, request.Format, request.Filter)
	if err != nil {
		return SubscriptionSource{}, err
	}
	var content []byte
	var contentFormat string
	filterChanged := !sameStrings(settings.filter.Include, current.Filter.Include) || !sameStrings(settings.filter.Exclude, current.Filter.Exclude)
	if request.Content != nil {
		if current.Kind != store.SubscriptionSourceFile {
			return SubscriptionSource{}, ErrSourceKindMismatch
		}
		reading, err := ReadSource([]byte(*request.Content), settings.format, settings.filter)
		if err != nil {
			return SubscriptionSource{}, err
		}
		if err := application.ensurePoolCapacity(ctx, current.ID, len(reading.Kept())); err != nil {
			return SubscriptionSource{}, err
		}
		content, contentFormat = []byte(*request.Content), string(reading.Format)
	} else if filterChanged {
		stored, err := application.database.SubscriptionSourceContent(ctx, current.ID)
		if err != nil {
			return SubscriptionSource{}, err
		}
		reading, err := ReadSource(stored.Content, SourceFormat(stored.Format), settings.filter)
		var parseErr *SourceParseError
		switch {
		case errors.As(err, &parseErr) && parseErr.Code == SourceErrorNodeNotConvertible, errors.As(err, &parseErr) && parseErr.Code == SourceErrorInvalidNodeName:
			return SubscriptionSource{}, err
		case err == nil:
			if err := application.ensurePoolCapacity(ctx, current.ID, len(reading.Kept())); err != nil {
				return SubscriptionSource{}, err
			}
		}
		// Content that no longer reads keeps its nodes; synchronization
		// records why.
	}
	next := current.NextRefreshAt
	if current.Kind == store.SubscriptionSourceURL && settings.refreshMinutes != current.RefreshMinutes {
		last := current.ContentAt
		if current.LastAttemptAt != nil {
			last = *current.LastAttemptAt
		}
		next = nextRefresh(settings.refreshMinutes, last)
	}
	stored, err := application.database.UpdateSubscriptionSource(ctx, store.UpdateSubscriptionSourceInput{
		ID: current.ID, Name: request.Name, URL: settings.url, RefreshMinutes: settings.refreshMinutes,
		Format: string(settings.format), Filter: storeSourceFilter(settings.filter), NextRefreshAt: next,
		Content: content, ContentFormat: contentFormat, ContentAt: application.now().UTC(),
		ExpectedUpdatedAt: expected, UpdatedAt: application.nextSubscriptionUpdateTime(expected),
	})
	if err != nil {
		return SubscriptionSource{}, err
	}
	application.PoolInputsChanged(ctx)
	return application.SubscriptionSource(ctx, stored.ID)
}

// RefreshSubscriptionSource fetches a URL source now. A failed fetch or parse
// keeps the current content and nodes and records the failure.
func (application *Service) RefreshSubscriptionSource(ctx context.Context, sourceID string) (result SubscriptionSource, err error) {
	defer func() {
		application.telemetry.RecordOperation(ctx, "subscription.refresh", "Subscription refresh", err, telemetry.OperationLogContext{})
	}()
	source, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return SubscriptionSource{}, err
	}
	if source.Kind != store.SubscriptionSourceURL {
		return SubscriptionSource{}, ErrSourceKindMismatch
	}
	return application.refreshSource(ctx, source)
}

func (application *Service) refreshSource(ctx context.Context, source store.SubscriptionSource) (SubscriptionSource, error) {
	content, err := application.fetchSource(ctx, source.URL)
	var reading SourceReading
	if err == nil {
		reading, err = ReadSource(content, SourceFormat(source.Format), sourceFilter(source.Filter))
	}
	if err == nil {
		err = application.ensurePoolCapacity(ctx, source.ID, len(reading.Kept()))
	}
	now := application.now().UTC()
	next := nextRefresh(source.RefreshMinutes, now)
	if err != nil {
		if ctx.Err() != nil {
			return SubscriptionSource{}, err
		}
		if _, recordErr := application.database.RecordSubscriptionSourceFailure(ctx, source.ID, sourceErrorCode(err), now, next); recordErr != nil {
			return SubscriptionSource{}, errors.Join(err, recordErr)
		}
		return SubscriptionSource{}, err
	}
	if _, err := application.database.ReplaceSubscriptionSourceContent(ctx, store.SubscriptionSourceContentUpdate{
		ID: source.ID, Content: content, ContentFormat: string(reading.Format), At: now, NextRefreshAt: next,
	}); err != nil {
		return SubscriptionSource{}, err
	}
	application.PoolInputsChanged(ctx)
	return application.SubscriptionSource(ctx, source.ID)
}

// RefreshDueSubscriptionSources refreshes every URL source whose next
// refresh is due.
func (application *Service) RefreshDueSubscriptionSources(ctx context.Context) error {
	due, err := application.database.DueSubscriptionSources(ctx, application.now().UTC())
	if err != nil {
		return err
	}
	for _, id := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		source, err := application.database.GetSubscriptionSource(ctx, id)
		if err != nil {
			continue
		}
		_, err = application.refreshSource(ctx, source)
		application.telemetry.RecordOperation(ctx, "subscription.refresh", "Subscription source refresh", err, telemetry.OperationLogContext{})
	}
	return nil
}

// DeleteSubscriptionSource deletes a source; its nodes leave the pool, every
// channel and grant with it.
func (application *Service) DeleteSubscriptionSource(ctx context.Context, sourceID string, expected time.Time) error {
	if err := application.database.DeleteSubscriptionSource(ctx, strings.TrimSpace(sourceID), expected); err != nil {
		return err
	}
	application.PoolInputsChanged(ctx)
	return nil
}

// PreviewSubscriptionSource reads content without saving anything: a stored
// source's content, a URL's content or the given content, filtered by the
// given rules. Content that does not read answers with its error.
func (application *Service) PreviewSubscriptionSource(ctx context.Context, request SubscriptionSourcePreviewRequest) (SubscriptionSourcePreview, error) {
	given := 0
	for _, present := range []bool{request.SourceID != "", request.URL != "", request.Content != nil} {
		if present {
			given++
		}
	}
	if given != 1 {
		return SubscriptionSourcePreview{}, &SourceFieldError{Path: "source_id", Code: "exactly_one_of_source_id_url_content"}
	}
	if request.Filter == nil {
		return SubscriptionSourcePreview{}, &SourceFieldError{Path: "filter", Code: "required"}
	}
	if err := request.Filter.Validate(); err != nil {
		return SubscriptionSourcePreview{}, err
	}
	format := request.Format
	if format != "" && !ValidSourceFormat(format, true) {
		return SubscriptionSourcePreview{}, &SourceFieldError{Path: "format", Code: "invalid_value"}
	}
	var content []byte
	switch {
	case request.SourceID != "":
		stored, err := application.database.SubscriptionSourceContent(ctx, request.SourceID)
		if err != nil {
			return SubscriptionSourcePreview{}, err
		}
		content = stored.Content
		if format == "" {
			format = SourceFormat(stored.Format)
		}
	case request.URL != "":
		if len(request.URL) > 4096 {
			return SubscriptionSourcePreview{}, &SourceFieldError{Path: "url", Code: "invalid_url"}
		}
		var err error
		if content, err = application.fetchSource(ctx, request.URL); err != nil {
			return SubscriptionSourcePreview{}, err
		}
	default:
		content = []byte(*request.Content)
	}
	if format == "" {
		format = SourceFormatAuto
	}
	reading, err := ReadSource(content, format, *request.Filter)
	preview := SubscriptionSourcePreview{Format: reading.Format, Items: make([]SubscriptionSourcePreviewItem, 0, len(reading.Items))}
	for _, item := range reading.Items {
		preview.Items = append(preview.Items, SubscriptionSourcePreviewItem{Location: item.Location, Name: item.Name, Protocol: item.Protocol, Kept: item.Kept})
	}
	var parseErr *SourceParseError
	if errors.As(err, &parseErr) {
		preview.Error = parseErr
	} else if err != nil {
		return SubscriptionSourcePreview{}, err
	}
	return preview, nil
}

// fetchSource fetches a URL source's content under the panel's source
// network policy, at most three at a time.
func (application *Service) fetchSource(ctx context.Context, url string) ([]byte, error) {
	select {
	case application.refreshSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-application.refreshSlots }()
	effective, err := application.settings.EffectiveSettings(ctx)
	if err != nil {
		return nil, err
	}
	return FetchSource(ctx, url, effective.Subscription.PrivateSourceCIDRs)
}

// ensurePoolCapacity fails when sourceID contributing kept nodes, in place
// of its current ones, would grow the pool beyond its limit.
func (application *Service) ensurePoolCapacity(ctx context.Context, sourceID string, kept int) error {
	pool, err := application.database.SubscriptionPool(ctx)
	if err != nil {
		return err
	}
	others := 0
	for _, node := range pool {
		if sourceID == "" || node.SourceID != sourceID {
			others++
		}
	}
	if others+kept > MaximumNodes {
		return fmt.Errorf("%w: the pool would hold more than %d nodes", store.ErrSubscriptionLimitExceeded, MaximumNodes)
	}
	return nil
}

func storeSourceFilter(filter SourceFilter) store.SubscriptionSourceFilter {
	return store.SubscriptionSourceFilter{Include: nonNil(filter.Include), Exclude: nonNil(filter.Exclude)}
}

func applicationSubscriptionSource(value store.SubscriptionSource) SubscriptionSource {
	source := SubscriptionSource{
		ID: value.ID, Name: value.Name, Kind: value.Kind, URL: value.URL, Format: SourceFormat(value.Format),
		Filter:    SourceFilter{Include: nonNil(value.Filter.Include), Exclude: nonNil(value.Filter.Exclude)},
		NodeCount: value.NodeCount, ContentFormat: SourceFormat(value.ContentFormat), ContentAt: value.ContentAt,
		LastAttemptAt: store.CloneTime(value.LastAttemptAt), LastError: value.LastError,
		NextRefreshAt: store.CloneTime(value.NextRefreshAt), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.Kind == store.SubscriptionSourceURL {
		minutes := value.RefreshMinutes
		source.RefreshMinutes = &minutes
	}
	return source
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
