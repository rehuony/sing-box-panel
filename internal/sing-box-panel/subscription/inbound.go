// SPDX-License-Identifier: GPL-3.0-or-later

// Inbound conversion turns a node's inbounds into the outbounds of its
// subscription nodes, by the inbound family of the core's release line.
package subscription

import (
	"errors"
)

var (
	ErrUnsupportedCoreVersion     = errors.New("unsupported sing-box version for inbound conversion")
	ErrAmbiguousInboundCredential = errors.New("ambiguous sing-box inbound credential")
	ErrInvalidPublicHost          = errors.New("invalid subscription public host")
)

type InboundRequest struct {
	FinalStartupJSON []byte
	PublicHost       string
}

type InboundResult struct {
	Nodes       []Node
	Diagnostics []ConversionDiagnostic
}
