// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestDownloadProgressTracksWrittenBytes(t *testing.T) {
	t.Parallel()
	const body = "binary data"
	for _, test := range []struct {
		name          string
		metadataSize  int64
		contentLength int64
		wantTotal     int64
	}{
		{"known size", int64(len(body)), int64(len(body)), int64(len(body))},
		{"HTTP size only", 0, int64(len(body)), int64(len(body))},
		{"metadata size only", int64(len(body)), -1, int64(len(body))},
		{"unknown size", 0, -1, 0},
		{"incorrect metadata", 1, int64(len(body)), int64(len(body))},
	} {
		t.Run(test.name, func(t *testing.T) {
			updater := New(Options{HTTPClient: &http.Client{Transport: progressTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Request: req, ContentLength: test.contentLength,
					Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(body)))}, nil
			})}})
			var destination bytes.Buffer
			var events []Progress
			digest, err := updater.downloadFile(t.Context(), asset{BrowserDownloadURL: "https://example.com/binary", Size: test.metadataSize}, &destination, maxBinaryBytes, func(progress Progress) {
				if progress.Stage != StageDownload || progress.Downloaded != int64(destination.Len()) {
					t.Fatalf("progress=%+v written=%d", progress, destination.Len())
				}
				events = append(events, progress)
			})
			if err != nil || destination.String() != body || digest != sha256.Sum256([]byte(body)) {
				t.Fatalf("body=%q digest=%x error=%v", destination.String(), digest, err)
			}
			if len(events) != len(body)+2 || events[0].Downloaded != 0 || events[0].Complete {
				t.Fatalf("missing initial or streaming events: %+v", events)
			}
			for i, event := range events[1 : len(events)-1] {
				if event.Downloaded != int64(i+1) || event.Total != test.wantTotal || event.Complete {
					t.Fatalf("stream event=%+v", event)
				}
			}
			last := events[len(events)-1]
			if !last.Complete || last.Downloaded != int64(len(body)) || last.Total != int64(len(body)) {
				t.Fatalf("completion=%+v", last)
			}
		})
	}
}

func TestDownloadFailureNeverReportsComplete(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		body    io.Reader
		maximum int64
		want    error
	}{
		{"truncated", io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(io.ErrUnexpectedEOF)), maxBinaryBytes, io.ErrUnexpectedEOF},
		{"oversized", strings.NewReader("oversized"), 4, ErrReleaseInvalid},
		{"empty", strings.NewReader(""), maxBinaryBytes, ErrReleaseInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			updater := New(Options{HTTPClient: &http.Client{Transport: progressTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Request: req, ContentLength: -1, Body: io.NopCloser(test.body)}, nil
			})}})
			_, err := updater.downloadFile(t.Context(), asset{BrowserDownloadURL: "https://example.com/binary"}, io.Discard, test.maximum, func(progress Progress) {
				if progress.Complete {
					t.Fatal("failed download reported completion")
				}
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v, want %v", err, test.want)
			}
		})
	}
}

func TestDownloadProgressHonorsCancellationDuringTransfer(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "partial")
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	updater := New(Options{HTTPClient: server.Client(), LatestReleaseURL: server.URL})
	var downloaded int64
	_, err := updater.downloadFile(ctx, asset{BrowserDownloadURL: server.URL}, io.Discard, maxBinaryBytes, func(progress Progress) {
		if progress.Complete {
			t.Fatal("cancelled transfer reported completion")
		}
		if progress.Downloaded > 0 {
			downloaded = progress.Downloaded
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || downloaded != int64(len("partial")) {
		t.Fatalf("downloaded=%d error=%v", downloaded, err)
	}
}

type progressTransport func(*http.Request) (*http.Response, error)

func (transport progressTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}
