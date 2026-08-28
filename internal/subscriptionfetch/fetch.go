// SPDX-License-Identifier: GPL-3.0-or-later

// Package subscriptionfetch performs bounded, SSRF-resistant remote
// subscription retrieval. It never logs or reflects a source URL.
package subscriptionfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	MaximumBodyBytes = 4 << 20
	MaximumRedirects = 5
	RequestTimeout   = 20 * time.Second
)

var (
	ErrInvalidURL       = errors.New("subscription source URL is invalid")
	ErrAddressDenied    = errors.New("subscription source address is denied")
	ErrTooManyRedirects = errors.New("subscription source has too many redirects")
	ErrBodyTooLarge     = errors.New("subscription source body is too large")
	ErrRequestFailed    = errors.New("subscription source request failed")
)

type resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// Fetch retrieves one complete candidate using strict redirect and address
// validation. Private addresses are allowed only when explicitly covered by
// allowedCIDRs.
func Fetch(ctx context.Context, rawURL string, allowedCIDRs []string) ([]byte, error) {
	allowed, err := parseAllowedCIDRs(allowedCIDRs)
	if err != nil {
		return nil, err
	}
	parsed, err := parseSourceURL(rawURL)
	if err != nil {
		return nil, err
	}
	policy := &networkPolicy{resolver: net.DefaultResolver, allowed: allowed}
	if err := policy.validateURL(ctx, parsed); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    true,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   1,
		IdleConnTimeout:       10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DialContext:           policy.dialContext,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   RequestTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > MaximumRedirects {
				return ErrTooManyRedirects
			}
			return policy.validateURL(request.Context(), request.URL)
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrInvalidURL
	}
	request.Header.Set("Accept", "application/json, application/yaml, text/yaml, text/plain, */*;q=0.1")
	request.Header.Set("User-Agent", "sing-box-panel/subscription-fetch")
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, ErrAddressDenied) {
			return nil, ErrAddressDenied
		}
		if errors.Is(err, ErrTooManyRedirects) {
			return nil, ErrTooManyRedirects
		}
		return nil, ErrRequestFailed
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("%w: http_status_%d", ErrRequestFailed, response.StatusCode)
	}
	limited := io.LimitReader(response.Body, MaximumBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, ErrRequestFailed
	}
	if len(body) > MaximumBodyBytes {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

type networkPolicy struct {
	resolver resolver
	allowed  []*net.IPNet
}

func (policy *networkPolicy) validateURL(ctx context.Context, target *url.URL) error {
	if target == nil || target.User != nil && (target.User.Username() == "" && target.User.String() != "") {
		return ErrInvalidURL
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return ErrInvalidURL
	}
	if target.Hostname() == "" || target.Fragment != "" || target.Opaque != "" {
		return ErrInvalidURL
	}
	if port := target.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return ErrInvalidURL
		}
	}
	addresses, err := policy.lookup(ctx, target.Hostname())
	if err != nil {
		return ErrRequestFailed
	}
	for _, address := range addresses {
		if !policy.permitted(address) {
			return ErrAddressDenied
		}
	}
	return nil
}

func (policy *networkPolicy) dialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalidURL
	}
	addresses, err := policy.lookup(ctx, host)
	if err != nil {
		return nil, ErrRequestFailed
	}
	for _, candidate := range addresses {
		if !policy.permitted(candidate) {
			return nil, ErrAddressDenied
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	var lastErr error
	for _, candidate := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, ErrRequestFailed
	}
	return nil, ErrRequestFailed
}

func (policy *networkPolicy) lookup(ctx context.Context, host string) ([]net.IP, error) {
	if address := net.ParseIP(strings.Trim(host, "[]")); address != nil {
		return []net.IP{address}, nil
	}
	resolved, err := policy.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(resolved) == 0 {
		return nil, ErrRequestFailed
	}
	addresses := make([]net.IP, 0, len(resolved))
	for _, value := range resolved {
		if value.IP != nil {
			addresses = append(addresses, value.IP)
		}
	}
	if len(addresses) == 0 {
		return nil, ErrRequestFailed
	}
	return addresses, nil
}

func (policy *networkPolicy) permitted(address net.IP) bool {
	for _, allowed := range policy.allowed {
		if allowed.Contains(address) {
			return true
		}
	}
	return !address.IsLoopback() && !address.IsPrivate() && !address.IsLinkLocalUnicast() &&
		!address.IsLinkLocalMulticast() && !address.IsUnspecified() && !address.IsMulticast()
}

func parseAllowedCIDRs(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, errors.New("invalid private source CIDR")
		}
		result = append(result, network)
	}
	return result, nil
}

func parseSourceURL(raw string) (*url.URL, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 8192 {
		return nil, ErrInvalidURL
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, ErrInvalidURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrInvalidURL
	}
	if parsed.Host == "" || parsed.Hostname() == "" || parsed.Opaque != "" || parsed.Fragment != "" {
		return nil, ErrInvalidURL
	}
	return parsed, nil
}
