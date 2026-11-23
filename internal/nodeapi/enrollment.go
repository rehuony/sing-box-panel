// SPDX-License-Identifier: GPL-3.0-or-later

package nodeapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	pathpkg "path"
	"regexp"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// TokenSize is the number of random bytes in an enrollment token.
const TokenSize = 32

const enrollmentCodePrefix = "sbp1."

var ErrEnrollmentCode = errors.New("enrollment code is malformed")

// EnrollmentCode is what an administrator copies to a node to enroll it:
// where the panel is, how to trust it and the one-time token.
type EnrollmentCode struct {
	// Endpoint is the host:port that nodes dial.
	Endpoint string `json:"endpoint"`
	// Path is the URL path prefix of the panel's node service, the panel's
	// base path; empty serves it at the root.
	Path string `json:"path,omitempty"`
	// Pin is the PublicKeyPin of the panel's TLS certificate. Empty
	// selects the system's trusted certificate authorities instead.
	Pin string `json:"pin,omitempty"`
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
	if ValidEndpoint(code.Endpoint) != nil || (code.Path != "" && ValidPathPrefix(code.Path) != nil) ||
		(code.Pin != "" && !validDigest(code.Pin)) || !validDigest(code.Token) {
		return EnrollmentCode{}, ErrEnrollmentCode
	}
	return code, nil
}

// ValidEndpoint checks a host:port address that nodes can dial.
func ValidEndpoint(endpoint string) error {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /\\@?#") {
		return errors.New("host is invalid")
	}
	return nil
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

// PublicKeyPin returns the pin of a certificate's public key: the SHA-256 of
// its DER SubjectPublicKeyInfo in unpadded base64url.
func PublicKeyPin(subjectPublicKeyInfo []byte) string {
	digest := sha256.Sum256(subjectPublicKeyInfo)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// TokenHash is the digest the panel stores for an enrollment token.
func TokenHash(token string) [32]byte {
	return sha256.Sum256([]byte(token))
}

// validDigest reports whether value is 32 bytes in unpadded base64url, the
// encoding of pins and tokens.
func validDigest(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}
