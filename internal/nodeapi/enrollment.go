// SPDX-License-Identifier: GPL-3.0-or-later

package nodeapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	pathpkg "path"
	"regexp"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// TokenSize is the number of random bytes in an enrollment token.
const TokenSize = 32

const enrollmentCodePrefix = "sbp2."

var ErrEnrollmentCode = errors.New("enrollment code is malformed")

// EnrollmentCode is what an administrator copies to a node to enroll it:
// where the panel is and the one-time token.
type EnrollmentCode struct {
	// URL is the panel's base URL, which ValidPanelURL accepts; the node
	// protocol's endpoints sit under it.
	URL string `json:"url"`
	// Token is the one-time enrollment token, TokenSize random bytes in
	// unpadded base64url.
	Token string `json:"token"`
}

// String encodes the code as one copyable word.
func (code EnrollmentCode) String() string {
	encoded, _ := json.Marshal(code) // Only strings are encoded.
	return enrollmentCodePrefix + base64.RawURLEncoding.EncodeToString(encoded)
}

// ParseEnrollmentCode decodes and validates a code produced by String.
func ParseEnrollmentCode(raw string) (EnrollmentCode, error) {
	payload, ok := strings.CutPrefix(strings.TrimSpace(raw), enrollmentCodePrefix)
	if !ok || len(payload) > 4096 {
		return EnrollmentCode{}, ErrEnrollmentCode
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return EnrollmentCode{}, ErrEnrollmentCode
	}
	var code EnrollmentCode
	if err := jsonstrict.Decode(decoded, 4096, &code); err != nil {
		return EnrollmentCode{}, ErrEnrollmentCode
	}
	if ValidPanelURL(code.URL) != nil || !validDigest(code.Token) {
		return EnrollmentCode{}, ErrEnrollmentCode
	}
	return code, nil
}

// ValidPanelURL checks a panel base URL that nodes connect to: an https
// origin, optionally followed by the panel's base path, or the same over
// plain http to a loopback address. Node traffic never crosses a network
// unencrypted, so plain http reaches only a panel on the node's own host.
func ValidPanelURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.RawPath != "" {
		return errors.New("must be a URL without credentials, query or fragment")
	}
	host := parsed.Hostname()
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /\\@?#") {
		return errors.New("host is invalid")
	}
	if port := parsed.Port(); port != "" {
		if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
			return errors.New("port must be between 1 and 65535")
		}
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !LoopbackHost(host) {
			return errors.New("plain http reaches only a panel on this host; use https")
		}
	default:
		return errors.New("scheme must be https")
	}
	if parsed.Path != "" && ValidPathPrefix(parsed.Path) != nil {
		return errors.New("path must be the panel's base path")
	}
	return nil
}

// LoopbackHost reports whether host, a host name or IP address, names this
// host's loopback interface.
func LoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var pathPrefixPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]+$`)

// ValidPathPrefix checks a URL path prefix such as the panel's base path: a
// normalized absolute path of unreserved characters without a trailing slash.
func ValidPathPrefix(prefix string) error {
	if !pathPrefixPattern.MatchString(prefix) || strings.HasSuffix(prefix, "/") ||
		strings.Contains(prefix, "//") || pathpkg.Clean(prefix) != prefix {
		return errors.New("must be a normalized URL path containing only unreserved characters")
	}
	return nil
}

// TokenHash is the digest the panel stores for an enrollment token.
func TokenHash(token string) [32]byte {
	return sha256.Sum256([]byte(token))
}

// validDigest reports whether value is 32 bytes in unpadded base64url, the
// encoding of tokens.
func validDigest(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}
