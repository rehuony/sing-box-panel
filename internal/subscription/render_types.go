// SPDX-License-Identifier: GPL-3.0-or-later

// Rendering emits public client subscriptions from one final sing-box startup
// document. Callers must pass bytes frozen in an activation bundle.
package subscription

import (
	"errors"
	"fmt"
)

const (
	// MaximumStartupBytes bounds the final startup document accepted by Render.
	MaximumStartupBytes = MaximumDocumentBytes
	maximumOutbounds    = MaximumNodes
	maximumFilters      = 10_000
)

var (
	ErrInvalidStartup = errors.New("invalid final startup JSON")
	ErrInvalidChannel = errors.New("invalid subscription channel")
)

// RenderFormat identifies a public subscription wire format.
type RenderFormat string

const (
	RenderFormatSingBox RenderFormat = "sing-box"
	RenderFormatMihomo  RenderFormat = "mihomo"
	RenderFormatLoon    RenderFormat = "loon"
)

// RenderChannel is the complete renderer input controlled by a subscription
// channel. Exclusions are exact and case-sensitive, matching sing-box tags and
// outbound types rather than accepting expressions.
type RenderChannel struct {
	Format       RenderFormat
	ExcludeTags  []string
	ExcludeTypes []string
}

// RenderDiagnostic identifies an omitted node by its zero-based position in the
// frozen startup document. It does not repeat its tag, address, or credentials.
type RenderDiagnostic struct {
	Collection Collection     `json:"collection"`
	ItemIndex  int            `json:"item_index"`
	Format     RenderFormat   `json:"format"`
	Code       DiagnosticCode `json:"code"`
}

// RenderResult owns the rendered public bytes and stable diagnostics.
type RenderResult struct {
	Format      RenderFormat       `json:"format"`
	MediaType   string             `json:"media_type"`
	Content     []byte             `json:"content"`
	NodeCount   int                `json:"node_count"`
	Diagnostics []RenderDiagnostic `json:"diagnostics"`
}

// RenderValidationError exposes a stable reason code without reflecting input data.
type RenderValidationError struct {
	scope error
	code  string
}

func (err *RenderValidationError) Error() string {
	return fmt.Sprintf("%v: %s", err.scope, err.code)
}

func (err *RenderValidationError) Unwrap() error {
	return err.scope
}

// Code returns the stable validation failure code.
func (err *RenderValidationError) Code() string {
	if err == nil {
		return ""
	}
	return err.code
}

func invalidStartup(code string) error {
	return &RenderValidationError{scope: ErrInvalidStartup, code: code}
}

func invalidChannel(code string) error {
	return &RenderValidationError{scope: ErrInvalidChannel, code: code}
}
