// SPDX-License-Identifier: GPL-3.0-or-later

// Package subscription renders public client subscriptions from one final
// sing-box startup document. It is deliberately independent of persistence,
// networking, and canonical configuration: callers must pass bytes frozen in
// an activation bundle.
package subscription

import (
	"errors"
	"fmt"
)

const (
	// MaximumStartupBytes bounds the final startup document accepted by Render.
	MaximumStartupBytes = 4 << 20
	maximumDepth        = 64
	maximumValues       = 200_000
	maximumOutbounds    = 10_000
	maximumFilters      = 10_000
)

var (
	ErrInvalidStartup = errors.New("invalid final startup JSON")
	ErrInvalidChannel = errors.New("invalid subscription channel")
)

// Format identifies a public subscription wire format.
type Format string

const (
	FormatSingBox Format = "sing-box"
	FormatMihomo  Format = "mihomo"
	FormatLoon    Format = "loon"
)

// DiagnosticCode is stable machine-readable evidence that one outbound could
// not be published. Codes intentionally contain no input values or secrets.
type DiagnosticCode string

const (
	DiagnosticInvalidOutbound      DiagnosticCode = "invalid_outbound"
	DiagnosticInvalidMetadata      DiagnosticCode = "invalid_metadata"
	DiagnosticDuplicateTag         DiagnosticCode = "duplicate_tag"
	DiagnosticUnsupportedType      DiagnosticCode = "unsupported_type"
	DiagnosticUnsupportedOption    DiagnosticCode = "unsupported_option"
	DiagnosticUnsupportedTransport DiagnosticCode = "unsupported_transport"
	DiagnosticUnsupportedTLS       DiagnosticCode = "unsupported_tls"
	DiagnosticUnsupportedNetwork   DiagnosticCode = "unsupported_network"
	DiagnosticUnresolvedDependency DiagnosticCode = "unresolved_dependency"
	DiagnosticInvalidRequiredField DiagnosticCode = "invalid_required_field"
)

// Channel is the complete renderer input controlled by a subscription
// channel. Exclusions are exact and case-sensitive, matching sing-box tags and
// outbound types rather than accepting expressions.
type Channel struct {
	Format       Format
	ExcludeTags  []string
	ExcludeTypes []string
}

// Collection identifies the final startup array that supplied a node.
type Collection string

const (
	CollectionOutbounds Collection = "outbounds"
	CollectionEndpoints Collection = "endpoints"
)

// Diagnostic identifies an omitted node by its zero-based position in the
// frozen startup document. It does not repeat its tag, address, or credentials.
type Diagnostic struct {
	Collection Collection     `json:"collection"`
	ItemIndex  int            `json:"item_index"`
	Format     Format         `json:"format"`
	Code       DiagnosticCode `json:"code"`
}

// Result owns the rendered public bytes and stable diagnostics.
type Result struct {
	Format      Format       `json:"format"`
	MediaType   string       `json:"media_type"`
	Content     []byte       `json:"content"`
	NodeCount   int          `json:"node_count"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// ValidationError exposes a stable reason code without reflecting input data.
type ValidationError struct {
	scope error
	code  string
}

func (err *ValidationError) Error() string {
	return fmt.Sprintf("%v: %s", err.scope, err.code)
}

func (err *ValidationError) Unwrap() error {
	return err.scope
}

// Code returns the stable validation failure code.
func (err *ValidationError) Code() string {
	if err == nil {
		return ""
	}
	return err.code
}

func invalidStartup(code string) error {
	return &ValidationError{scope: ErrInvalidStartup, code: code}
}

func invalidChannel(code string) error {
	return &ValidationError{scope: ErrInvalidChannel, code: code}
}
