// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func strictCoreQuery(w http.ResponseWriter, request *http.Request, allowed ...string) (url.Values, bool) {
	if len(request.URL.RawQuery) > maximumCoreQueryBytes {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "The query string is too large.")
		return nil, false
	}
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "The query string is malformed.")
		return nil, false
	}
	permitted := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		permitted[name] = struct{}{}
	}
	for name, values := range query {
		if _, ok := permitted[name]; !ok || len(values) != 1 || values[0] == "" {
			writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "Query parameters must be recognized, non-empty, and occur at most once.")
			return nil, false
		}
	}
	return query, true
}

func optionalStrictBool(w http.ResponseWriter, request *http.Request, query url.Values, name string) (bool, bool) {
	values, present := query[name]
	if !present {
		return false, true
	}
	switch values[0] {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", name+" must be true or false.")
		return false, false
	}
}

func requireEmptyCoreBody(w http.ResponseWriter, request *http.Request) bool {
	body, err := readBoundedBody(request, maximumCoreEmptyRequestBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "request_too_large", "Request rejected", "The request body is too large.")
		return false
	}
	if len(strings.TrimSpace(string(body))) != 0 {
		writeProblem(w, request, http.StatusUnprocessableEntity, "request_body_not_allowed", "Request invalid", "This operation does not accept a request body.")
		return false
	}
	return true
}

func validOptionalExactVersion(value string, requireNonZero bool) bool {
	if value == "" {
		return true
	}
	version, err := coreartifact.ParseExactVersion(value)
	return err == nil && (!requireNonZero || !version.IsZero())
}
func validOptionalArchitecture(value string) bool {
	return value == "" || value == "amd64" || value == "arm64"
}
func validOptionalCoreArtifactSource(value string) bool {
	return value == "" || value == string(store.CoreArtifactSourceOfficial) || value == string(store.CoreArtifactSourceUserVerified)
}
func validOptionalCoreArtifactVerification(value string) bool {
	return value == "" || value == string(store.CoreArtifactVerified) || value == string(store.CoreArtifactRevoked) || value == string(store.CoreArtifactQuarantined)
}

func validOptionalVariant(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validCoreArtifactID(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCoreImportRequest(request application.CoreImportRequest) bool {
	if !filepath.IsAbs(request.SourcePath) || filepath.Clean(request.SourcePath) != request.SourcePath {
		return false
	}
	digest, err := coreartifact.ParseSHA256(request.SHA256)
	if err != nil || digest.IsZero() {
		return false
	}
	version, err := coreartifact.ParseExactVersion(request.ExactVersion)
	if err != nil || version.IsZero() {
		return false
	}
	architecture := coreartifact.Architecture(request.Architecture)
	if architecture != coreartifact.ArchitectureAMD64 && architecture != coreartifact.ArchitectureARM64 {
		return false
	}
	variant := coreartifact.Variant(request.Variant)
	if variant == "" {
		variant = coreartifact.VariantPlain
	}
	source, err := coreartifact.NewUserSource(request.SourceDescription)
	if err != nil {
		return false
	}
	_, err = coreartifact.NewIdentity(source, digest, coreartifact.OperatingSystemLinux, architecture, variant, version)
	return err == nil
}
