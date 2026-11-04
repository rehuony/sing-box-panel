// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrUnsafeURL rejects download URLs outside the allowed public hosts.
	ErrUnsafeURL = errors.New("artifact download URL is not allowed")
	// ErrDownloadTooLarge rejects archives above the size limit.
	ErrDownloadTooLarge = errors.New("artifact download exceeds its size limit")
)

// DownloadError is a failed official artifact download. Its text is the
// stable code alone: the cause, such as a transport error, can quote the
// signed download URL.
type DownloadError struct {
	Code  string
	Cause error
}

func (err *DownloadError) Error() string { return "download artifact: " + err.Code }

func (err *DownloadError) Unwrap() error { return err.Cause }

func downloadFailure(code string, cause error) error { return &DownloadError{Code: code, Cause: cause} }

var defaultDownloadHosts = []string{
	"github.com",
	"objects.githubusercontent.com",
	"release-assets.githubusercontent.com",
}

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type SafeDownloaderOptions struct {
	Resolver         Resolver
	Dialer           ContextDialer
	AllowedHosts     []string
	Timeout          time.Duration
	MaximumRedirects int

	// roundTripper is deliberately package-private. Production callers cannot
	// bypass the DNS-to-dial binding while tests can still replace HTTP I/O.
	roundTripper http.RoundTripper
}

type SafeDownloader struct {
	resolver     Resolver
	dialer       ContextDialer
	allowedHosts map[string]struct{}
	timeout      time.Duration
	redirects    int
	client       *http.Client
}

func NewSafeDownloader(options SafeDownloaderOptions) (*SafeDownloader, error) {
	if options.Resolver == nil {
		options.Resolver = net.DefaultResolver
	}
	if options.Dialer == nil {
		options.Dialer = &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	}
	if options.Timeout <= 0 {
		options.Timeout = 2 * time.Minute
	}
	if options.MaximumRedirects <= 0 {
		options.MaximumRedirects = 5
	}
	if options.Timeout > 10*time.Minute || options.MaximumRedirects > 10 {
		return nil, downloadFailure("invalid_limits", nil)
	}
	hosts := options.AllowedHosts
	if len(hosts) == 0 {
		hosts = defaultDownloadHosts
	}
	allowed := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSuffix(host, "."))
		if host == "" || net.ParseIP(host) != nil || strings.ContainsAny(host, "/:@") {
			return nil, downloadFailure("invalid_allowlist", nil)
		}
		allowed[host] = struct{}{}
	}
	downloader := &SafeDownloader{
		resolver:     options.Resolver,
		dialer:       options.Dialer,
		allowedHosts: allowed,
		timeout:      options.Timeout,
		redirects:    options.MaximumRedirects,
	}
	transport := options.roundTripper
	if transport == nil {
		base := http.DefaultTransport.(*http.Transport).Clone()
		base.Proxy = nil
		base.DialContext = downloader.secureDialContext
		base.ResponseHeaderTimeout = 30 * time.Second
		transport = base
	}
	downloader.client = &http.Client{
		Transport: transport,
		Timeout:   options.Timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > downloader.redirects {
				return downloadFailure("redirect_limit", ErrUnsafeURL)
			}
			if err := downloader.validateEndpoint(request.Context(), request.URL); err != nil {
				return err
			}
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			request.Header.Del("Referer")
			return nil
		},
	}
	return downloader, nil
}

func (downloader *SafeDownloader) Download(
	ctx context.Context,
	rawURL string,
	destination io.Writer,
	maximumBytes int64,
) (int64, error) {
	if downloader == nil || downloader.client == nil || destination == nil || maximumBytes <= 0 {
		return 0, downloadFailure("invalid_request", nil)
	}
	if ctx == nil {
		return 0, downloadFailure("nil_context", nil)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, downloadFailure("unsafe_url", ErrUnsafeURL)
	}
	operationContext, cancel := context.WithTimeout(ctx, downloader.timeout)
	defer cancel()
	if err := downloader.validateEndpoint(operationContext, parsed); err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(operationContext, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return 0, downloadFailure("request_build", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "sing-box-panel")
	response, err := downloader.client.Do(request)
	if err != nil {
		return 0, downloadFailure("request_failed", err)
	}
	if response == nil || response.Body == nil {
		return 0, downloadFailure("invalid_response", nil)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, downloadFailure("http_status", nil)
	}
	if response.ContentLength > maximumBytes {
		return 0, downloadFailure("body_too_large", ErrDownloadTooLarge)
	}
	// The request context bounds the read; one extra byte detects an
	// oversized body.
	written, err := io.Copy(destination, io.LimitReader(response.Body, maximumBytes+1))
	if err == nil && written > maximumBytes {
		err = ErrDownloadTooLarge
	}
	if err != nil {
		return written, downloadFailure("body_read", err)
	}
	return written, nil
}

func (downloader *SafeDownloader) validateEndpoint(ctx context.Context, endpoint *url.URL) error {
	if endpoint == nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.Fragment != "" ||
		endpoint.Hostname() == "" || (endpoint.Port() != "" && endpoint.Port() != "443") {
		return downloadFailure("unsafe_url", ErrUnsafeURL)
	}
	host := strings.ToLower(strings.TrimSuffix(endpoint.Hostname(), "."))
	if _, allowed := downloader.allowedHosts[host]; !allowed {
		return downloadFailure("host_not_allowed", ErrUnsafeURL)
	}
	if strings.Contains(endpoint.Path, "\\") {
		return downloadFailure("unsafe_path", ErrUnsafeURL)
	}
	if _, err := downloader.resolvePublic(ctx, host); err != nil {
		return err
	}
	return nil
}

func (downloader *SafeDownloader) secureDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, downloadFailure("dial_network", ErrUnsafeURL)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, downloadFailure("dial_address", ErrUnsafeURL)
	}
	if port != "443" {
		return nil, downloadFailure("dial_port", ErrUnsafeURL)
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, allowed := downloader.allowedHosts[host]; !allowed {
		return nil, downloadFailure("dial_host_not_allowed", ErrUnsafeURL)
	}
	addresses, err := downloader.resolvePublic(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastError error
	for _, address := range addresses {
		connection, err := downloader.dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return connection, nil
		}
		lastError = err
	}
	return nil, downloadFailure("dial_failed", lastError)
}

func (downloader *SafeDownloader) resolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	addresses, err := downloader.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, downloadFailure("dns_failed", err)
	}
	if len(addresses) == 0 {
		return nil, downloadFailure("dns_empty", ErrUnsafeURL)
	}
	result := make([]netip.Addr, 0, len(addresses))
	for _, resolved := range addresses {
		address, valid := netip.AddrFromSlice(resolved.IP)
		if !valid {
			return nil, downloadFailure("dns_invalid", ErrUnsafeURL)
		}
		address = address.Unmap()
		if !isPublicAddress(address) {
			return nil, downloadFailure("dns_non_public", ErrUnsafeURL)
		}
		result = append(result, address)
	}
	return result, nil
}

var deniedAddressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

var allocatedGlobalIPv6 = netip.MustParsePrefix("2000::/3")

func isPublicAddress(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	if address.Is6() && !allocatedGlobalIPv6.Contains(address) {
		return false
	}
	for _, prefix := range deniedAddressPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
