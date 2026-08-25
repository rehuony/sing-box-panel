// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestSafeDownloaderDownloadsBoundedPublicArtifact(t *testing.T) {
	t.Parallel()
	resolver := resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
	})
	downloader, err := NewSafeDownloader(SafeDownloaderOptions{
		Resolver: resolver,
		roundTripper: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Hostname() != "github.com" {
				t.Fatalf("request host = %q", request.URL.Hostname())
			}
			return downloadResponse(http.StatusOK, "artifact"), nil
		}),
	})
	if err != nil {
		t.Fatalf("NewSafeDownloader: %v", err)
	}
	var destination bytes.Buffer
	written, err := downloader.Download(context.Background(), "https://github.com/SagerNet/sing-box/releases/download/v1.0.0/file.tar.gz", &destination, 64)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if written != 8 || destination.String() != "artifact" {
		t.Fatalf("Download = (%d, %q), want (8, artifact)", written, destination.String())
	}
}

func TestSafeDownloaderRejectsUnsafeEndpointsBeforeHTTP(t *testing.T) {
	t.Parallel()
	requests := 0
	publicResolver := resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
	})
	downloader, err := NewSafeDownloader(SafeDownloaderOptions{
		Resolver: publicResolver,
		roundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests++
			return downloadResponse(http.StatusOK, "unexpected"), nil
		}),
	})
	if err != nil {
		t.Fatalf("NewSafeDownloader: %v", err)
	}
	unsafeURLs := []string{
		"http://github.com/file",
		"https://example.com/file",
		"https://user:password@github.com/file",
		"https://github.com:8443/file",
		"https://github.com/file#fragment",
		"https://127.0.0.1/file",
	}
	for _, rawURL := range unsafeURLs {
		if _, err := downloader.Download(context.Background(), rawURL, io.Discard, 64); !errors.Is(err, ErrUnsafeURL) {
			t.Fatalf("Download(%q) error = %v, want ErrUnsafeURL", rawURL, err)
		}
	}
	if requests != 0 {
		t.Fatalf("unsafe URLs reached HTTP transport %d times", requests)
	}
}

func TestSafeDownloaderRejectsPrivateOrMixedDNS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		addresses []net.IPAddr
	}{
		{name: "private", addresses: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}},
		{name: "loopback", addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}},
		{name: "metadata", addresses: []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}},
		{name: "mixed rebinding answer", addresses: []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}, {IP: net.ParseIP("192.168.1.1")}}},
		{name: "documentation", addresses: []net.IPAddr{{IP: net.ParseIP("2001:db8::1")}}},
		{name: "NAT64 translated private", addresses: []net.IPAddr{{IP: net.ParseIP("64:ff9b::7f00:1")}}},
		{name: "6to4 embedded loopback", addresses: []net.IPAddr{{IP: net.ParseIP("2002:7f00:1::")}}},
		{name: "deprecated site local", addresses: []net.IPAddr{{IP: net.ParseIP("fec0::1")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			downloader, err := NewSafeDownloader(SafeDownloaderOptions{
				Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) { return test.addresses, nil }),
				roundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatalf("unsafe DNS answer reached HTTP transport")
					return nil, nil
				}),
			})
			if err != nil {
				t.Fatalf("NewSafeDownloader: %v", err)
			}
			if _, err := downloader.Download(context.Background(), "https://github.com/file", io.Discard, 64); !errors.Is(err, ErrUnsafeURL) {
				t.Fatalf("Download error = %v, want ErrUnsafeURL", err)
			}
		})
	}
}

func TestSafeDownloaderRevalidatesRedirectDNS(t *testing.T) {
	t.Parallel()
	resolver := resolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "github.com" {
			return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	requests := 0
	downloader, err := NewSafeDownloader(SafeDownloaderOptions{
		Resolver: resolver,
		roundTripper: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			response := downloadResponse(http.StatusFound, "")
			response.Header.Set("Location", "https://release-assets.githubusercontent.com/private")
			return response, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewSafeDownloader: %v", err)
	}
	if _, err := downloader.Download(context.Background(), "https://github.com/file", io.Discard, 64); !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("redirect error = %v, want ErrUnsafeURL", err)
	}
	if requests != 1 {
		t.Fatalf("HTTP requests = %d, want only initial request", requests)
	}
}

func TestSafeDownloaderRedirectLimitCountsFollowedRedirects(t *testing.T) {
	t.Parallel()
	resolver := resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
	})
	t.Run("one redirect allowed", func(t *testing.T) {
		requests := 0
		downloader, err := NewSafeDownloader(SafeDownloaderOptions{
			Resolver: resolver, MaximumRedirects: 1,
			roundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					response := downloadResponse(http.StatusFound, "")
					response.Header.Set("Location", "https://release-assets.githubusercontent.com/artifact")
					return response, nil
				}
				return downloadResponse(http.StatusOK, "artifact"), nil
			}),
		})
		if err != nil {
			t.Fatalf("NewSafeDownloader: %v", err)
		}
		if _, err := downloader.Download(context.Background(), "https://github.com/start", io.Discard, 64); err != nil || requests != 2 {
			t.Fatalf("one redirect = (%d requests, %v), want success", requests, err)
		}
	})
	t.Run("second redirect rejected", func(t *testing.T) {
		requests := 0
		downloader, err := NewSafeDownloader(SafeDownloaderOptions{
			Resolver: resolver, MaximumRedirects: 1,
			roundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				response := downloadResponse(http.StatusFound, "")
				response.Header.Set("Location", "https://release-assets.githubusercontent.com/again")
				return response, nil
			}),
		})
		if err != nil {
			t.Fatalf("NewSafeDownloader: %v", err)
		}
		if _, err := downloader.Download(context.Background(), "https://github.com/start", io.Discard, 64); !errors.Is(err, ErrUnsafeURL) || requests != 2 {
			t.Fatalf("second redirect = (%d requests, %v), want ErrUnsafeURL", requests, err)
		}
	})
}

func TestSafeDownloaderBoundsBodyAndPreservesCancellation(t *testing.T) {
	t.Parallel()
	resolver := resolverFunc(func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
	})
	downloader, err := NewSafeDownloader(SafeDownloaderOptions{
		Resolver: resolver,
		roundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return downloadResponse(http.StatusOK, strings.Repeat("x", 65)), nil
		}),
	})
	if err != nil {
		t.Fatalf("NewSafeDownloader: %v", err)
	}
	if _, err := downloader.Download(context.Background(), "https://github.com/file", io.Discard, 64); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized Download error = %v, want ErrTooLarge", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := downloader.Download(cancelled, "https://github.com/file", io.Discard, 64); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Download error = %v, want context.Canceled", err)
	}
}

func TestSafeDownloaderFailureTextDoesNotLeakSignedURL(t *testing.T) {
	t.Parallel()
	downloader, err := NewSafeDownloader(SafeDownloaderOptions{
		Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, nil
		}),
		roundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("signed-secret")
		}),
	})
	if err != nil {
		t.Fatalf("NewSafeDownloader: %v", err)
	}
	_, err = downloader.Download(context.Background(), "https://github.com/file?token=signed-secret", io.Discard, 64)
	if err == nil || strings.Contains(err.Error(), "signed-secret") {
		t.Fatalf("safe Download error = %v", err)
	}
}

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (resolver resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return resolver(ctx, host)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func downloadResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}
