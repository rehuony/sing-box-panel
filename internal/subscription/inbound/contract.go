// SPDX-License-Identifier: GPL-3.0-or-later

// Package inbound dispatches exact-version sing-box inbound converters.
package inbound

import (
	"errors"

	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

var (
	ErrUnsupportedCoreVersion     = errors.New("unsupported sing-box version for inbound conversion")
	ErrAmbiguousInboundCredential = errors.New("ambiguous sing-box inbound credential")
	ErrInvalidPublicHost          = errors.New("invalid subscription public host")
)

type Request struct {
	FinalStartupJSON []byte
	PublicHost       string
}

type Result struct {
	Nodes       []node.Node
	Diagnostics []node.ConversionDiagnostic
}

type Converter interface {
	ExactVersion() string
	Convert(Request) (Result, error)
}
