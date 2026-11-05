// SPDX-License-Identifier: GPL-3.0-or-later

package webauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// NormalizeOrigin validates an HTTP Origin value and returns its canonical
// scheme and authority. Paths, credentials, queries, and fragments are never
// part of an origin and are rejected instead of silently discarded.
func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("origin must not be empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("origin scheme must be http or https")
	}
	if parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Path != "" ||
		parsed.RawPath != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin must contain only a scheme and authority")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || strings.Contains(host, "%") {
		return "", errors.New("origin host is invalid")
	}
	port := parsed.Port()
	if port != "" {
		numericPort, err := strconv.Atoi(port)
		if err != nil || numericPort < 1 || numericPort > 65535 {
			return "", errors.New("origin port must be between 1 and 65535")
		}
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	authority := host
	if port != "" {
		authority = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	return scheme + "://" + authority, nil
}

// SameOrigin reports whether the request comes from the trusted origin: the
// configured one when set, else the request's own scheme and host. Forwarded
// headers are never trusted.
func SameOrigin(request *http.Request, trusted string) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return false
	}
	actual, err := NormalizeOrigin(origin)
	if err != nil {
		return false
	}
	if trusted != "" {
		return actual == trusted
	}
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	expected, err := NormalizeOrigin(scheme + "://" + request.Host)
	return err == nil && actual == expected
}

// ChangesState reports whether a request method may change state and so
// needs the CSRF token and a same-origin request.
func ChangesState(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

// TokenEqual compares two secrets in constant time.
func TokenEqual(left, right string) bool {
	leftHash, rightHash := sha256.Sum256([]byte(left)), sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

// SetSecurityHeaders forbids sniffing, framing, referrers and every source
// but the API's own.
func SetSecurityHeaders(header http.Header) {
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
}
