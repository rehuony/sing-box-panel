// SPDX-License-Identifier: GPL-3.0-or-later

// Package render renders public client subscriptions from one final
// sing-box startup document. It is deliberately independent of persistence,
// networking, and canonical configuration: callers must pass bytes frozen in
// an activation bundle.
package render

import (
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/subscription/document"
	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

const (
	// MaximumStartupBytes bounds the final startup document accepted by Render.
	MaximumStartupBytes = document.MaximumBytes
	maximumOutbounds    = node.MaximumNodes
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
type DiagnosticCode = node.DiagnosticCode

const (
	DiagnosticInvalidOutbound      = node.DiagnosticInvalidOutbound
	DiagnosticInvalidMetadata      = node.DiagnosticInvalidMetadata
	DiagnosticDuplicateTag         = node.DiagnosticDuplicateTag
	DiagnosticUnsupportedType      = node.DiagnosticUnsupportedType
	DiagnosticUnsupportedOption    = node.DiagnosticUnsupportedOption
	DiagnosticUnsupportedTransport = node.DiagnosticUnsupportedTransport
	DiagnosticUnsupportedTLS       = node.DiagnosticUnsupportedTLS
	DiagnosticUnsupportedNetwork   = node.DiagnosticUnsupportedNetwork
	DiagnosticUnresolvedDependency = node.DiagnosticUnresolvedDependency
	DiagnosticInvalidRequiredField = node.DiagnosticInvalidRequiredField
	DiagnosticUnpublishableInbound = node.DiagnosticUnpublishableInbound
	DiagnosticAmbiguousCredential  = node.DiagnosticAmbiguousCredential
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
type Collection = node.Collection

const (
	CollectionOutbounds = node.CollectionOutbounds
	CollectionEndpoints = node.CollectionEndpoints
	CollectionInbounds  = node.CollectionInbounds
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
