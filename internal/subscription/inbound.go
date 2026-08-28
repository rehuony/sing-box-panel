// SPDX-License-Identifier: GPL-3.0-or-later

// Inbound conversion dispatches exact sing-box profiles to reviewed behavior.
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

type InboundConverter interface {
	ExactVersion() string
	Convert(InboundRequest) (InboundResult, error)
}
